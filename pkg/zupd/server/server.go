package server

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/cldmnky/ksdns/pkg/zupd/config"
	"github.com/cldmnky/ksdns/pkg/zupd/file"
	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"
)

type Server struct {
	host       string
	port       string
	rTimeout   time.Duration
	wTimeout   time.Duration
	tsigSecret map[string]string
	config     *config.Config
}

func NewServer(config *config.Config) *Server {
	return &Server{
		host:       config.Addr,
		port:       config.Port,
		rTimeout:   5 * time.Second,
		wTimeout:   5 * time.Second,
		tsigSecret: map[string]string{".": config.Secret},
		config:     config,
	}
}

func (s *Server) Addr() string {
	return fmt.Sprintf("%s:%s", s.host, s.port)
}

func (s *Server) start(ds *dns.Server) {
	log.Info(fmt.Sprintf("Start %s listener on %s", ds.Net, s.Addr()))
	err := ds.ListenAndServe()
	if err != nil {
		log.Error(fmt.Sprintf("Start %s listener on %s failed:%s", ds.Net, s.Addr(), err.Error()))
	}
}

func (s *Server) Run(ctx context.Context) {
	handler := NewHandler(s.config)
	tcpHandler := dns.NewServeMux()
	tcpHandler.HandleFunc(".", handler.serveDNS)
	udpHandler := dns.NewServeMux()
	udpHandler.HandleFunc(".", handler.serveDNS)
	tcpServer := &dns.Server{Addr: s.Addr(),
		Net:          "tcp",
		Handler:      tcpHandler,
		ReadTimeout:  s.rTimeout,
		WriteTimeout: s.wTimeout,
	}

	udpServer := &dns.Server{Addr: s.Addr(),
		Net:          "udp",
		Handler:      udpHandler,
		UDPSize:      65535,
		ReadTimeout:  s.rTimeout,
		WriteTimeout: s.wTimeout,
	}

	go s.start(udpServer)
	go s.start(tcpServer)

}

type Handler struct {
	Storage *file.File
}

func NewHandler(config *config.Config) *Handler {
	// setup storage for zone files
	storage, err := file.NewFile(config)
	if err != nil {
		log.Error(err)
		log.Fatalf("Failed to setup storage for zone files")
		// TODO error handling
	}
	return &Handler{
		Storage: storage,
	}
}

func (h *Handler) Name() string {
	return "zupd"
}

func (h *Handler) serveDNS(w dns.ResponseWriter, req *dns.Msg) {
	var (
		remote net.IP
		proto  string
	)
	switch v := w.RemoteAddr().(type) {
	case *net.UDPAddr:
		remote = v.IP
		proto = "udp"
	case *net.TCPAddr:
		remote = v.IP
		proto = "tcp"
	}

	log.Info(fmt.Sprintf("Received opcode %v proto %s request for %s from %s", req.Opcode, proto, req.Question[0].Name, remote))
	log.Info(fmt.Sprintf("Received request: %s", req.String()))

	m := new(dns.Msg)
	m.SetReply(req)

	// Handle AXFR
	if req.Question[0].Qtype == dns.TypeAXFR {
		// check if it's a tcp request
		if proto != "tcp" {
			m.SetRcode(req, dns.RcodeRefused)
			w.WriteMsg(m)
			log.Debugf("Received AXFR request from %s, but not a TCP request", remote)
			return
		}
		log.Debugf("Received AXFR request for %s from %s", req.Question[0].Name, remote)
		// get the zone from storage
		z := h.Storage.GetZone(req.Question[0].Name)
		if z == nil {
			m.SetRcode(req, dns.RcodeNameError)
			w.WriteMsg(m)
			log.Debugf("Received AXFR request for %s from %s, but zone not found", req.Question[0].Name, remote)
			return
		}
		var pchan <-chan []dns.RR
		var err error
		pchan, err = z.Transfer(0)
		if err != nil {
			log.Errorf("Failed to transfer zone %s: %s", req.Question[0].Name, err)
			m.SetRcode(req, dns.RcodeServerFailure)
		}
		// Send response to client
		ch := make(chan *dns.Envelope)
		tr := new(dns.Transfer)
		if req.IsTsig() != nil {
			// TODO: check if the key is valid
			tr.TsigSecret = map[string]string{".": "123456"}
		}
		errCh := make(chan error)
		go func() {
			if err := tr.Out(w, req, ch); err != nil {
				errCh <- err
			}
			close(errCh)
		}()
		rrs := []dns.RR{}
		l := 0
		var soa *dns.SOA
		for records := range pchan {
			if x, ok := records[0].(*dns.SOA); ok && soa == nil {
				soa = x
			}
			rrs = append(rrs, records...)
		}
		if len(rrs) > 0 {
			select {
			case ch <- &dns.Envelope{RR: rrs}:
			case err := <-errCh:
				log.Errorf("Failed to send response to client: %s", err)
			}
			l += len(rrs)
		}

		close(ch)     // Even though we close the channel here, we still have
		err = <-errCh // to wait before we can return and close the connection.
		if err != nil {
			log.Errorf("Failed to send response to client: %s", err)
		}

		logserial := uint32(0)
		if soa != nil {
			logserial = soa.Serial
		}
		log.Info(fmt.Sprintf("Sent %d records to %s, serial %d", l, remote, logserial))
	}

	switch req.Opcode {
	case dns.OpcodeQuery:
		m.Extra = make([]dns.RR, 1)
		m.Extra[0] = &dns.TXT{Hdr: dns.RR_Header{Name: m.Question[0].Name, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 0}, Txt: []string{"Hello world"}}
	case dns.OpcodeUpdate:
		if req.IsTsig() != nil {
			status := w.TsigStatus()
			if status != nil {
				log.Errorf("TSIG error: %s", status.Error())
				m.SetRcode(req, dns.RcodeRefused)
				w.WriteMsg(m)
				return
			} else {
				log.Info("TSIG OK")
			}
		} else {
			log.Errorf("Received update request without TSIG")
			m.SetRcode(req, dns.RcodeRefused)
			w.WriteMsg(m)
			return
		}
		// check that we are the authority for the zone
		zone := req.Question[0].Name
		z := h.Storage.GetZone(zone)
		if z == nil {
			log.Errorf("Received update request for zone %s, but zone not found", zone)
			m.SetRcode(req, dns.RcodeRefused)
			w.WriteMsg(m)
			return
		}

		changed := false

		for _, question := range req.Question {
			for _, rr := range req.Ns {
				header := rr.Header()
				if _, ok := dns.IsDomainName(rr.Header().Name); ok {
					switch updatePacketType(header) {
					case "insert":
						log.Infof("Insert (%s) %s", question.Name, rr.String())
						if err := h.Storage.Insert(zone, rr); err != nil {
							log.Errorf("Failed to insert record: %s", err)
							m.SetRcode(req, dns.RcodeServerFailure)
							w.WriteMsg(m)
							return
						}
						log.Debugf("Inserted %s", rr.String())
						changed = true
					case "remove":
						log.Infof("Remove (%s) %s", question.Name, rr.String())
						h.Storage.Delete(zone, rr)
						changed = true
					case "removeName":
						log.Infof("RemoveName (%s) %s", question.Name, rr.String())
						// not implemented
						m.SetRcode(req, dns.RcodeNotImplemented)
						w.WriteMsg(m)
						return
					case "removeRRSet":
						log.Infof("RemoveRRSet (%s) %s", question.Name, rr.String())
						// not implemented
						m.SetRcode(req, dns.RcodeNotImplemented)
						w.WriteMsg(m)
						return
					default:
						log.Info(fmt.Sprintf("Received unknown request for %s", header.Name))
						m.SetRcode(req, dns.RcodeNotImplemented)
					}
				}
			}
		}
		if changed {
			// Update SOA serial
			apex, err := z.ApexIfDefined()
			if err != nil {
				log.Errorf("Failed to get SOA record: %s", err)
				m.SetRcode(req, dns.RcodeServerFailure)
				w.WriteMsg(m)
				return
			}
			for _, rr := range apex {
				// get the Soa record
				if soa, ok := rr.(*dns.SOA); ok {
					soa.Serial++
					if err := z.Insert(soa); err != nil {
						log.Errorf("Failed to update SOA record: %s", err)
						m.SetRcode(req, dns.RcodeServerFailure)
						w.WriteMsg(m)
						return
					}
					log.Debugf("Updated SOA serial to %d", soa.Serial)
				}
			}
		}
	default:
		m.SetRcode(req, dns.RcodeNotImplemented)
	}
	w.WriteMsg(m)
}

func updatePacketType(h *dns.RR_Header) string {
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
