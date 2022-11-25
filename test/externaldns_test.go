package test

import (
	"context"
	"testing"
	"time"

	plugintest "github.com/coredns/coredns/plugin/test"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider/rfc2136"
)

const (
	fakeZone       = "example.org."
	fakeTsigKey    = "example.org."
	fakeTsigSecret = "IwBTJx9wrDp4Y1RyC3H0gA=="
)

func TestExternalDNS(t *testing.T) {
	name, rm, err := plugintest.TempFile(".", exampleOrg)
	require.NoError(t, err)
	defer rm()

	corefile := `example.org:1053 {
		debug
		bind 127.0.0.1
		dynamicupdate ` + name + ` {
			reload 5s
		}
		transfer {
			to * 
			to 192.168.1.1
		}
		tsig {
			secret ` + fakeTsigKey + ` ` + fakeTsigSecret + `
			require all
		}
	}`

	i, udp, tcp, err := CoreDNSServerAndPorts(corefile)
	require.NoError(t, err)
	defer i.Stop()

	t.Log("udp:", udp, "tcp:", tcp)
	// Log directives and plugins loaded.

	provider, err := rfc2136.NewRfc2136Provider(
		"127.0.0.1",
		1053,
		fakeZone,
		false,
		fakeTsigKey,
		fakeTsigSecret,
		"hmac-sha256",
		true,
		endpoint.DomainFilter{
			Filters: []string{},
		},
		false,
		time.Duration(time.Second),
		false,
		"",
		"",
		"",
		10,
		nil,
	)
	require.NoError(t, err)
	recs, err := provider.Records(context.Background())
	require.NoError(t, err)
	require.Len(t, recs, 6)

	p := &plan.Changes{
		Create: []*endpoint.Endpoint{
			{
				DNSName:    "foo.example.org",
				RecordType: "A",
				Targets:    []string{"1.2.3.4"},
				RecordTTL:  endpoint.TTL(400),
			},
			{
				DNSName:    "foo.example.org",
				RecordType: "TXT",
				Targets:    []string{"boom"},
			},
		},
		Delete: []*endpoint.Endpoint{
			{
				DNSName:    "vpn.example.org",
				RecordType: "A",
				Targets:    []string{"216.146.45.240"},
			},
			{
				DNSName:    "vpn.example.org",
				RecordType: "TXT",
				Targets:    []string{"boom2"},
			},
		},
	}

	err = provider.ApplyChanges(context.Background(), p)
	require.NoError(t, err)

	recs, err = provider.Records(context.Background())
	require.NoError(t, err)
	require.Len(t, recs, 8)

	// Lookup

	// New client
	c := new(dns.Client)
	c.TsigSecret = map[string]string{fakeTsigKey: fakeTsigSecret}
	m := new(dns.Msg)
	m.SetTsig(fakeTsigSecret, dns.HmacSHA256, 300, time.Now().Unix())
	m.SetQuestion("foo.example.org.", dns.TypeA)
	r, _, err := c.Exchange(m, udp)
	require.NoError(t, err)
	require.Len(t, r.Answer, 1)
	require.Equal(t, "foo.example.org.", r.Answer[0].Header().Name)
	require.Equal(t, uint32(400), r.Answer[0].Header().Ttl)

}
