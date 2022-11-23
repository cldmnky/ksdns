package dynamicupdate

import (
	"github.com/coredns/coredns/plugin/transfer"
	"github.com/miekg/dns"
)

// Transfer implements the transfer.Transfer interface.
func (d DynamicUpdate) Transfer(zone string, serial uint32) (<-chan []dns.RR, error) {
	z, ok := d.Zones.Z[zone]
	if !ok || z == nil {
		return nil, transfer.ErrNotAuthoritative
	}
	return z.Transfer(serial)
}
