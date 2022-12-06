package v1alpha1

import (
	"fmt"
	"net"
	"time"

	"github.com/coredns/coredns/plugin/pkg/dnsutil"
	"github.com/miekg/dns"
)

func newSOARecord(zone string) ([]dns.RR, error) {
	if zone == "" {
		return nil, fmt.Errorf("zone cannot be empty")
	}
	zone = dns.Fqdn(zone)
	ttl := uint32(30)
	Mbox := dnsutil.Join("hostmaster", zone)
	Ns := dnsutil.Join(nsName, zone)
	header := dns.RR_Header{Name: zone, Rrtype: dns.TypeSOA, Ttl: ttl, Class: dns.ClassINET}
	soa := &dns.SOA{Hdr: header,
		Mbox:    Mbox,
		Ns:      Ns,
		Serial:  uint32(time.Now().Unix()),
		Refresh: 7200,
		Retry:   1800,
		Expire:  86400,
		Minttl:  ttl,
	}
	return []dns.RR{soa}, nil
}

func newNSRecord(zone string, ip net.IP) (records, extra []dns.RR, err error) {
	if zone == "" {
		return nil, nil, fmt.Errorf("zone cannot be empty")
	}
	host := dns.Fqdn(dnsutil.Join(nsName, zone))
	zone = dns.Fqdn(zone)
	ttl := uint32(30)
	records = []dns.RR{
		&dns.NS{Hdr: dns.RR_Header{Name: zone, Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: ttl}, Ns: host},
	}
	extra = append(extra, newAddress(host, ttl, ip, dns.TypeA))
	return records, extra, nil
}

func newAddress(name string, ttl uint32, ip net.IP, what uint16) dns.RR {
	hdr := dns.RR_Header{Name: name, Rrtype: what, Class: dns.ClassINET, Ttl: ttl}

	if what == dns.TypeA {
		return &dns.A{Hdr: hdr, A: ip}
	}
	// Should always be dns.TypeAAAA
	return &dns.AAAA{Hdr: hdr, AAAA: ip}
}

func newA(name string, ttl uint32, ip net.IP) *dns.A {
	return &dns.A{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: ttl}, A: ip}
}

func newCNAME(name string, ttl uint32, target string) *dns.CNAME {
	return &dns.CNAME{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: ttl}, Target: dns.Fqdn(target)}
}

func newTXT(name string, ttl uint32, text string) *dns.TXT {
	return &dns.TXT{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: ttl}, Txt: split255(text)}
}

func newSRV(name string, ttl uint32, target string, weight uint16, priority, port uint16) *dns.SRV {
	host := dns.Fqdn(target)

	return &dns.SRV{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeSRV, Class: dns.ClassINET, Ttl: ttl},
		Priority: priority, Weight: weight, Port: port, Target: host}
}

// Split255 splits a string into 255 byte chunks.
func split255(s string) []string {
	if len(s) < 255 {
		return []string{s}
	}
	sx := []string{}
	p, i := 0, 255
	for {
		if i <= len(s) {
			sx = append(sx, s[p:i])
		} else {
			sx = append(sx, s[p:])
			break
		}
		p, i = p+255, i+255
	}

	return sx
}
