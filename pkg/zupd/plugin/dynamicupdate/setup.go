package dynamicupdate

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/file"
	"github.com/coredns/coredns/plugin/metrics"
	"github.com/coredns/coredns/plugin/pkg/upstream"
	"github.com/coredns/coredns/plugin/transfer"
	"github.com/coredns/kubeapi"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func init() { plugin.Register("dynamicupdate", setup) }

func setup(c *caddy.Controller) error {
	zones, err := fileparse(c)
	if err != nil {
		return plugin.Error("dynamicupdate", err)
	}
	d := DynamicUpdate{
		Zones: zones,
	}
	// get the transfer plugin, so we can send notifies and send notifies on startup as well.
	c.OnStartup(func() error {
		m := dnsserver.GetConfig(c).Handler("prometheus")
		if m != nil {
			d.metrics = m.(*metrics.Metrics)
		}
		t := dnsserver.GetConfig(c).Handler("transfer")
		if t != nil {
			d.transfer = t.(*transfer.Transfer)
		}
		c := dnsserver.GetConfig(c).Handler("kubeapi")
		if c != nil {
			cfg, err := c.(*kubeapi.KubeAPI).ClientConfig.ClientConfig()
			if err != nil {
				return err
			}
			d.client, err = client.New(cfg, client.Options{Scheme: clientgoscheme.Scheme})
			if err != nil {
				return err
			}
			// start controller
			go func() {
				if err := d.RunController(); err != nil {
					log.Errorf("Failed to run controller: %v", err)
				}
			}()
		}
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
		c.OnShutdown(z.OnShutdown)
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

func fileparse(c *caddy.Controller) (Zones, error) {
	z := make(map[string]*file.Zone)
	dz := make(map[string]*file.Zone)
	names := []string{}

	config := dnsserver.GetConfig(c)
	var openErr error
	reload := 1 * time.Minute

	for c.Next() {
		// dynamicupdate db.file [zones...]
		if !c.NextArg() {
			return Zones{}, c.ArgErr()
		}
		fileName := c.Val()

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
			z[origins[i]].ReloadInterval = reload
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
	return Zones{Z: z, Names: names, DynamicZones: dz}, nil
}
