package dynamicupdate

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/file"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

const (
	fakeZone       = "example.com."
	fakeTsigKey    = "example.com."
	fakeTsigSecret = "IwBTJx9wrDp4Y1RyC3H0gA=="
)

func TestDynamicUpdateAllowedTypes(t *testing.T) {
	zone, err := file.Parse(strings.NewReader(exampleOrg), exampleOrgZone, "stdin", 0)
	require.NoError(t, err)
	dynamicZone := file.NewZone(exampleOrgZone, "")
	d := DynamicUpdate{
		Zones: Zones{
			Z: map[string]*file.Zone{
				exampleOrgZone: zone,
			},
			DynamicZones: map[string]*file.Zone{
				exampleOrgZone: dynamicZone,
			},
			Names: []string{exampleOrgZone},
		},
		Next: test.ErrorHandler(),
	}
	ctx := context.TODO()

	// Test allowed types
	// A
	m := new(dns.Msg)
	m.SetUpdate("example.org.")
	m.SetTsig(fakeTsigKey, dns.HmacSHA256, 300, time.Now().Unix())
	m.Insert([]dns.RR{testRR(t, "insert.example.org 3600 IN A 127.0.0.1")})
	rec := dnstest.NewRecorder(&test.ResponseWriter{})
	code, err := d.ServeDNS(ctx, rec, m)
	require.NoError(t, err)
	require.Equal(t, dns.RcodeSuccess, code)
	// Lookup the record
	m = new(dns.Msg)
	m.SetQuestion("insert.example.org.", dns.TypeA)
	rec = dnstest.NewRecorder(&test.ResponseWriter{})
	code, err = d.ServeDNS(ctx, rec, m)
	require.NoError(t, err)
	require.Equal(t, dns.RcodeSuccess, code)
	require.Len(t, rec.Msg.Answer, 1)

	// AAAA
	m = new(dns.Msg)
	m.SetUpdate("example.org.")
	m.SetTsig(fakeTsigKey, dns.HmacSHA256, 300, time.Now().Unix())
	m.Insert([]dns.RR{testRR(t, "insert.example.org 3600 IN AAAA ::1")})
	rec = dnstest.NewRecorder(&test.ResponseWriter{})
	code, err = d.ServeDNS(ctx, rec, m)
	require.NoError(t, err)
	require.Equal(t, dns.RcodeSuccess, code)

	// CNAME
	m = new(dns.Msg)
	m.SetUpdate("example.org.")
	m.SetTsig(fakeTsigKey, dns.HmacSHA256, 300, time.Now().Unix())
	m.Insert([]dns.RR{testRR(t, "insert.example.org 3600 IN CNAME example.org.")})
	rec = dnstest.NewRecorder(&test.ResponseWriter{})
	code, err = d.ServeDNS(ctx, rec, m)
	require.NoError(t, err)
	require.Equal(t, dns.RcodeSuccess, code)

	// TXT
	m = new(dns.Msg)
	m.SetUpdate("example.org.")
	m.SetTsig(fakeTsigKey, dns.HmacSHA256, 300, time.Now().Unix())
	m.Insert([]dns.RR{testRR(t, "insert.example.org 3600 IN TXT \"test\"")})
	rec = dnstest.NewRecorder(&test.ResponseWriter{})
	code, err = d.ServeDNS(ctx, rec, m)
	require.NoError(t, err)
	require.Equal(t, dns.RcodeSuccess, code)

	// SRV
	m = new(dns.Msg)
	m.SetUpdate("example.org.")
	m.SetTsig(fakeTsigKey, dns.HmacSHA256, 300, time.Now().Unix())
	m.Insert([]dns.RR{testRR(t, "_sip._tcp.example.org. 3600 IN SRV 0 5 5060 sip.example.org.")})
	rec = dnstest.NewRecorder(&test.ResponseWriter{})
	code, err = d.ServeDNS(ctx, rec, m)
	require.NoError(t, err)
	require.Equal(t, dns.RcodeSuccess, code)

	// NS should be refused
	m = new(dns.Msg)
	m.SetUpdate("example.org.")
	m.SetTsig(fakeTsigKey, dns.HmacSHA256, 300, time.Now().Unix())
	m.Insert([]dns.RR{testRR(t, "insert.example.org 3600 IN NS ns1.example.org.")})
	rec = dnstest.NewRecorder(&test.ResponseWriter{})
	code, err = d.ServeDNS(ctx, rec, m)
	require.NoError(t, err)
	require.Equal(t, dns.RcodeRefused, code)

}

func testRR(t *testing.T, s string) dns.RR {
	r, err := dns.NewRR(s)
	require.NoError(t, err)
	return r
}
