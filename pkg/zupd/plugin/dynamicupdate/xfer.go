package dynamicupdate

import (
	"github.com/coredns/coredns/plugin/transfer"
	"github.com/miekg/dns"
)

// Transfer implements the transfer.Transfer interface.
func (d DynamicUpdate) Transfer(zone string, serial uint32) (<-chan []dns.RR, error) {
	z := d.Merge(zone)
	if z == nil {
		return nil, transfer.ErrNotAuthoritative
	}
	return z.Transfer(serial)
}
