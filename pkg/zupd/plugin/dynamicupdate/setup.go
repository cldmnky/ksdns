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

	rfc1035v1alpha1 "github.com/cldmnky/ksdns/api/v1alpha1"
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
	if err := d.NewManager(Cfg); err != nil {
		return plugin.Error("dynamicupdate", err)
	}

	d.Client = d.mgr.GetClient()

	//d.Client = d.mgr.GetClient()
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

	zones, err := d.setupController(c)
	if err != nil {
		return plugin.Error("dynamicupdate", err)
	}
	d.Zones = zones

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
		return d
	})

	return nil
}

func (d DynamicUpdate) setupController(c *caddy.Controller) (Zones, error) {
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
		log.Infof("Namespaces: %v", d.Namespaces)
	}
	clnt, err := client.New(Cfg, client.Options{})
	if err != nil {
		return Zones{}, err
	}
	for _, n := range d.Namespaces {
		// get all zones
		zones := &rfc1035v1alpha1.ZoneList{}
		if err := clnt.List(context.Background(), zones, client.InNamespace(n)); err != nil {
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
	}
	log.Debugf("Zones: %v", z)
	/*
			origins := plugin.OriginsFromArgsOrServerBlock(c.RemainingArgs(), c.ServerBlockKeys)
			if !filepath.IsAbs(fileName) && config.Root != "" {
				fileName = filepath.Join(config.Root, fileName)
			}

			reader, err := os.Open(filepath.Clean(fileName))
			if err != nil {
				openErr = err
			}

			err = func() error {
				defer reader.Close()

				for i := range origins {
					z[origins[i]] = file.NewZone(origins[i], fileName)
					dz[origins[i]] = file.NewZone(origins[i], "")
					if openErr == nil {
						reader.Seek(0, 0)
						zone, err := file.Parse(reader, origins[i], fileName, 0)
						if err != nil {
							return err
						}
						z[origins[i]] = zone
					}
					names = append(names, origins[i])
				}
				return nil
			}()

			if err != nil {
				return Zones{}, err
			}

			for c.NextBlock() {
				switch c.Val() {
				case "reload":
					t := c.RemainingArgs()
					if len(t) < 1 {
						return Zones{}, errors.New("reload duration value is expected")
					}
					d, err := time.ParseDuration(t[0])
					if err != nil {
						return Zones{}, plugin.Error("file", err)
					}
					reload = d
				case "upstream":
					// remove soon
					c.RemainingArgs()

				default:
					return Zones{}, c.Errf("unknown property '%s'", c.Val())
				}
			}

			for i := range origins {
				z[origins[i]].ReloadInterval = 0 * time.Second
				z[origins[i]].Upstream = upstream.New()
			}
		}

		if openErr != nil {
			if reload == 0 {
				// reload hasn't been set make this a fatal error
				return Zones{}, plugin.Error("file", openErr)
			}
			log.Warningf("Failed to open %q: trying again in %s", openErr, reload)
		}
	*/
	return Zones{Z: z, Names: names, DynamicZones: dz}, nil
}
