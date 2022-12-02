package dynamicupdate

import (
	"context"
	"fmt"
	"strings"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/file"
	"github.com/coredns/coredns/plugin/metrics"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/transfer"
	"github.com/miekg/dns"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rfc1035v1alpha1 "github.com/cldmnky/ksdns/pkg/zupd/api/v1alpha1"
)

var (
	log = clog.NewWithPlugin("dynamicupdate")
	Cfg = ctrl.GetConfigOrDie()
)

func init() {
	plugin.Register("dynamicupdate", setup)
}

func setup(c *caddy.Controller) error {
	d := DynamicUpdate{}

	client, err := client.New(Cfg, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return plugin.Error("dynamicupdate", err)
	}
	d.K8sClient = client

	zones, err := d.initialize(c)
	if err != nil {
		return plugin.Error("dynamicupdate", err)
	}
	d.Zones = &zones

	if err := d.NewManager(Cfg); err != nil {
		return plugin.Error("dynamicupdate", err)
	}

	ctx, stopManager := context.WithCancel(context.Background())

	c.OnStartup(func() error {
		m := dnsserver.GetConfig(c).Handler("prometheus")
		if m != nil {
			d.metrics = m.(*metrics.Metrics)
		} else {
			return plugin.Error("prometheus plugin is required", fmt.Errorf("must be enabled in Corefile"))
		}
		t := dnsserver.GetConfig(c).Handler("transfer")
		if t != nil {
			d.transfer = t.(*transfer.Transfer)
		} else {
			return plugin.Error("transfer plugin is required", fmt.Errorf("must be enabled in Corefile"))
		}
		return nil
	})

	c.OnStartup(func() error {
		// start controller
		go func() {
			if err := d.mgr.Start(ctx); err != nil {
				log.Errorf("Failed to run controller: %v", err)
			}
		}()
		return nil
	})

	c.OnShutdown(func() error {
		log.Infof("Shutting down dynamicupdate plugin")
		stopManager()
		return nil
	})

	c.OnShutdown(func() error {
		// stop zones
		d.OnShutdown()
		return nil
	})

	c.OnStartup(func() error {
		go func() {
			for _, n := range zones.Names {
				d.transfer.Notify(n)
			}
		}()
		return nil
	})

	c.OnRestartFailed(func() error {
		t := dnsserver.GetConfig(c).Handler("transfer")
		if t == nil {
			return nil
		}
		go func() {
			for _, n := range zones.Names {
				d.transfer.Notify(n)
			}
		}()
		return nil
	})

	for _, n := range zones.Names {
		z := zones.Z[n]
		c.OnStartup(func() error {
			z.StartupOnce.Do(func() { d.Reload(n, d.transfer) })
			return nil
		})
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		d.Next = next
		return &d
	})

	return nil
}

func (d *DynamicUpdate) initialize(c *caddy.Controller) (Zones, error) {
	z := make(map[string]*file.Zone)
	dz := make(map[string]*file.Zone)
	names := []string{}
	d.Namespaces = []string{}

	for c.Next() {
		// dynamicupdate [namespaces...]
		if !c.NextArg() {
			return Zones{}, c.ArgErr()
		}
		namespace := c.Val()
		if namespace == "*" {
			d.Namespaces = []string{}
		}
		d.Namespaces = append(d.Namespaces, namespace)
		for ok := c.NextArg(); ok; ok = c.NextArg() {
			d.Namespaces = append(d.Namespaces, c.Val())
		}
		log.Debugf("Namespaces: %v", d.Namespaces)
	}

	for _, n := range d.Namespaces {
		// get all zones
		// TODO check if namespace exists or is *
		zones := &rfc1035v1alpha1.ZoneList{}
		if err := d.K8sClient.List(context.Background(), zones, client.InNamespace(n)); err != nil {
			log.Errorf("Failed to list zones: %v", err)
			return Zones{}, err
		}
		for _, zone := range zones.Items {
			if _, ok := z[dns.Fqdn(zone.Name)]; !ok {
				parsedZone, err := file.Parse(strings.NewReader(zone.Spec.Zone), dns.Fqdn(zone.Name), "stdin", 0)
				if err != nil {
					log.Errorf("Failed to parse zone %s: %v", zone.Name, err)
					continue
				}
				z[dns.Fqdn(zone.Name)] = parsedZone
				dz[dns.Fqdn(zone.Name)] = file.NewZone(dns.Fqdn(zone.Name), "")
				names = append(names, dns.Fqdn(zone.Name))
			}
		}
		// Read status of zones, add to DynamicRRs
		for _, zone := range zones.Items {
			if _, ok := z[dns.Fqdn(zone.Name)]; ok {
				for _, rr := range zone.Status.DynamicRRs {
					newRR, err := dns.NewRR(rr.RR)
					if err != nil {
						log.Errorf("Failed to parse RR %s: %v", rr, err)
						continue
					}
					dz[dns.Fqdn(zone.Name)].Insert(newRR)
				}
			}
		}
	}
	return Zones{Z: z, Names: names, DynamicZones: dz}, nil
}
