package dynamicupdate

import (
	"github.com/cldmnky/ksdns/pkg/zupd/core/dnsserver"
	"github.com/coredns/caddy"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
)

var log = clog.NewWithPlugin("dynamicupdate")

func init() { plugin.Register("dynamicupdate", setup) }

func setup(c *caddy.Controller) error {
	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		return nil
	})

	return nil
}
