package dynamicupdate

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/file"
	"github.com/coredns/coredns/plugin/metrics"
	"github.com/coredns/coredns/plugin/transfer"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	rfc1035v1alpha1 "github.com/cldmnky/ksdns/api/v1alpha1"
)

// Types
type (
	// DynamicUpdate is a plugin that implements the file backend.
	DynamicUpdate struct {
		// Next plugin in the chain.
		Next plugin.Handler
		// Zones holds the configuration for the zones handled by this plugin.
		Zones *Zones
		// Namspaces holds the configuration for the namespaces handled by this plugin.
		Namespaces []string
		// transfer implements the transfer plugin.
		transfer *transfer.Transfer
		// metrics implements the metrics plugin.
		metrics *metrics.Metrics
		// K8sClient is the client used to communicate with the kubernetes API server.
		K8sClient client.Client
		// mgr is the manager used to run the controller.
		mgr manager.Manager

		// Client
		client.Client
		// Scheme
		Scheme *runtime.Scheme
	}

	Zones struct {
		Z            map[string]*file.Zone
		Names        []string
		DynamicZones map[string]*file.Zone
		sync.RWMutex
	}
)

func (z *Zones) DeleteZone(name string) {
	z.Lock()
	defer z.Unlock()
	delete(z.Z, name)
	delete(z.DynamicZones, name)
	// delete from names
	for i, n := range z.Names {
		if n == name {
			z.Names = append(z.Names[:i], z.Names[i+1:]...)
			break
		}
	}
}

// ServeDNS implements the plugin.Handler interface.
func (d *DynamicUpdate) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}
	qname := state.Name()
	zone := plugin.Zones(d.Zones.Names).Matches(qname)
	if zone == "" {
		return plugin.NextOrFailure(d.Name(), d.Next, ctx, w, r)
	}

	z, ok := d.Zones.Z[zone]
	if !ok || z == nil {
		return dns.RcodeServerFailure, nil
	}

	dz, ok := d.Zones.DynamicZones[zone]
	if !ok || dz == nil {
		return dns.RcodeServerFailure, nil
	}

	// If transfer is not loaded, we'll see these, answer with refused (no transfer allowed).
	if state.QType() == dns.TypeAXFR || state.QType() == dns.TypeIXFR {
		return dns.RcodeRefused, nil
	}
	// This is only for when we are a secondary zones. Drop the request.
	if r.Opcode == dns.OpcodeNotify {
		log.Infof("Dropping notify from %s for %s", state.IP(), zone)
		return dns.RcodeSuccess, nil
	}

	// Handle dynamic update
	if r.Opcode == dns.OpcodeUpdate {
		var (
			found     bool = false
			zoneObj   rfc1035v1alpha1.Zone
			soaSerial uint32 = uint32(time.Now().UnixMilli())
		)
		log.Infof("Handling dynamic update for %s", zone)
		dz.Lock()

		for range r.Question {
			for _, rr := range r.Ns {
				// Only allow TXT, CNAME, A, AAAA, and SRV records
				if rr.Header().Rrtype != dns.TypeTXT &&
					rr.Header().Rrtype != dns.TypeCNAME &&
					rr.Header().Rrtype != dns.TypeA &&
					rr.Header().Rrtype != dns.TypeAAAA &&
					rr.Header().Rrtype != dns.TypeSRV {
					log.Infof("Rejecting dynamic update for %s: %s", zone, rr.Header().String())
					dz.Unlock()
					return dns.RcodeRefused, nil
				}
				// Get the record
				h := rr.Header()
				if _, ok := dns.IsDomainName(h.Name); ok {
					switch updateType(h) {
					case "insert":
						log.Infof("Inserting %s", rr.String())
						if err := dz.Insert(rr); err != nil {
							log.Infof("Error inserting %s: %s", rr.String(), err.Error())
							dz.Unlock()
							return dns.RcodeServerFailure, nil
						}
						// get the zone from the cluster
						// update the zone with the new record
					case "remove":
						log.Infof("Removing %s", rr.String())
						dz.Delete(rr)
					default:
						log.Infof("Unknown update type for %s", rr.String())
						dz.Unlock()
						return dns.RcodeNotImplemented, nil
					}
				}
			}
		}
		dz.Unlock()
		dz.RLock()
		defer dz.RUnlock()
		// Get the zone
		for _, ns := range d.Namespaces {
			if err := d.K8sClient.Get(context.TODO(), client.ObjectKey{
				Namespace: ns,
				Name:      strings.TrimSuffix(zone, "."),
			}, &zoneObj); err != nil {
				continue
			}
			found = true
			break
		}
		if !found {
			log.Infof("Rejecting dynamic update for %s, object not found", zone)
			return dns.RcodeRefused, nil
		}
		// Update the zone
		zoneObj.Status.DynamicRRs = make([]rfc1035v1alpha1.DynamicRR, 0)
		for _, el := range dz.All() {
			for _, rr := range el.All() {
				zoneObj.Status.DynamicRRs = append(zoneObj.Status.DynamicRRs, rfc1035v1alpha1.DynamicRR{
					RR: rr.String(),
				})
			}
		}
		// set serial
		zoneObj.Status.Serial = soaSerial
		// Update the zone
		if err := d.K8sClient.Status().Update(context.TODO(), &zoneObj); err != nil {
			log.Infof("Error updating zone object: %s", err.Error())
			return dns.RcodeServerFailure, nil
		}
		z = d.Merge(zone)
		// Update SOA serial
		apex, err := z.ApexIfDefined()
		if err != nil {
			log.Errorf("Failed to get SOA record: %s", err)
			return dns.RcodeServerFailure, nil
		}
		for _, rr := range apex {
			// get the Soa record
			if soa, ok := rr.(*dns.SOA); ok {
				soa.Serial = soaSerial
				if err := z.Insert(soa); err != nil {
					log.Errorf("Failed to update SOA record: %s", err)
					return dns.RcodeServerFailure, nil
				}
				log.Infof("Updated SOA serial to %d", soa.Serial)
			}
		}

		// Notify other servers
		if d.transfer != nil {
			log.Infof("Notifying other  qservers of update")
			d.transfer.Notify(zone)
		}

		m := new(dns.Msg)
		m.SetReply(r)
		m.Authoritative = true
		if err := w.WriteMsg(m); err != nil {
			log.Infof("Error writing response: %s", err.Error())
			return dns.RcodeServerFailure, nil
		}
		// log message
		log.Infof("Dynamic update for %s from %s: %s", zone, state.IP(), m.String())
		return dns.RcodeSuccess, nil
	}
	z = d.Merge(zone)
	z.RLock()
	exp := z.Expired
	z.RUnlock()
	if exp {
		log.Errorf("Zone %s is expired", zone)
		return dns.RcodeServerFailure, nil
	}

	answer, ns, extra, result := z.Lookup(ctx, state, qname)
	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true
	m.Answer, m.Ns, m.Extra = answer, ns, extra

	switch result {
	case file.Success:
	case file.NoData:
	case file.NameError:
		m.Rcode = dns.RcodeNameError
	case file.Delegation:
		m.Authoritative = false
	case file.ServerFailure:
		// If the result is SERVFAIL and the answer is non-empty, then the SERVFAIL came from an
		// external CNAME lookup and the answer contains the CNAME with no target record. We should
		// write the CNAME record to the client instead of sending an empty SERVFAIL response.
		if len(m.Answer) == 0 {
			return dns.RcodeServerFailure, nil
		}
		//  The rcode in the response should be the rcode received from the target lookup. RFC 6604 section 3
		m.Rcode = dns.RcodeServerFailure
	}

	w.WriteMsg(m)
	return dns.RcodeSuccess, nil

}

func (d DynamicUpdate) Name() string { return "dynamicupdate" }

type serialErr struct {
	err    string
	zone   string
	origin string
	serial int64
}

func (s *serialErr) Error() string {
	return fmt.Sprintf("%s for origin %s in file %s, with %d SOA serial", s.err, s.origin, s.zone, s.serial)
}

func updateType(h *dns.RR_Header) string {
	switch h.Class {
	case dns.ClassINET:
		return "insert"
	case dns.ClassNONE:
		return "remove"
	case dns.ClassANY:
		if h.Rrtype == dns.TypeANY {
			return "removeName"
		} else {
			return "removeRRSet"
		}
	default:
		return "unknown"
	}
}
