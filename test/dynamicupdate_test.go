package test

import (
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestZoneEDNS0Lookup(t *testing.T) {
	t.Parallel()

	name, rm, err := test.TempFile(".", `$ORIGIN example.org.
@ 3600 IN SOA  sns.dns.icann.org. noc.dns.icann.org. (
        2017042745 ; serial
        7200       ; refresh (2 hours)
        3600       ; retry (1 hour)
        1209600    ; expire (2 weeks)
        3600       ; minimum (1 hour)
)
  3600 IN NS   a.iana-servers.net.
  3600 IN NS   b.iana-servers.net.
www    IN A    127.0.0.1
www    IN AAAA ::1
`)
	if err != nil {
		t.Fatalf("Failed to create zone: %s", err)
	}
	defer rm()

	// Corefile with for example without proxy section.
	corefile := `example.org:0 {
		dynamicupdate ` + name + `
	}`

	i, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeMX)
	m.SetEdns0(4096, true)

	r, err := dns.Exchange(m, udp)
	if err != nil {
		t.Fatalf("Could not exchange msg: %s", err)
	}
	if r.Rcode == dns.RcodeServerFailure {
		t.Fatalf("Rcode should not be dns.RcodeServerFailure")
	}
}

func TestZoneNoNS(t *testing.T) {
	t.Parallel()

	name, rm, err := test.TempFile(".", `$ORIGIN example.org.
@ 3600 IN SOA  sns.dns.icann.org. noc.dns.icann.org. (
		2017042745 ; serial
		7200       ; refresh (2 hours)
		3600       ; retry (1 hour)
		1209600    ; expire (2 weeks)
		3600       ; minimum (1 hour)
	)
www    IN A    127.0.0.1
www    IN AAAA ::1
`)
	if err != nil {
		t.Fatalf("Failed to create zone: %s", err)
	}
	defer rm()

	// Corefile with for example without proxy section.
	corefile := `example.org:0 {
		dynamicupdate ` + name + `
	}`
	i, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeMX)
	m.SetEdns0(4096, true)

	r, err := dns.Exchange(m, udp)
	if err != nil {
		t.Fatalf("Could not exchange msg: %s", err)
	}
	if r.Rcode == dns.RcodeServerFailure {
		t.Fatalf("Rcode should not be dns.RcodeServerFailure")
	}
}

func TestZoneSRVAdditional(t *testing.T) {
	t.Parallel()

	name, rm, err := test.TempFile(".", exampleOrg2)
	if err != nil {
		t.Fatalf("Failed to create zone: %s", err)
	}
	defer rm()

	// Corefile with for example without proxy section.
	corefile := `example.org:10053 {
		kubeapi
		bind 127.0.0.1
		dynamicupdate ` + name + `
	}`

	i, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	m := new(dns.Msg)
	m.SetQuestion("service.example.org.", dns.TypeSRV)
	resp, err := dns.Exchange(m, udp)
	if err != nil {
		t.Fatalf("Expected to receive reply, but didn't: %s", err)
	}

	// There should be 2 A records in the additional section.
	if len(resp.Extra) != 2 {
		t.Fatalf("Expected 2 RR in additional section got %d", len(resp.Extra))
	}
}
