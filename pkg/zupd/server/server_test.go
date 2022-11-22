package server

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/cldmnky/ksdns/pkg/zupd/config"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider/rfc2136"

	log "github.com/sirupsen/logrus"
)

const (
	fakeZone       = "example.com."
	fakeTsigKey    = "example.com."
	fakeTsigSecret = "IwBTJx9wrDp4Y1RyC3H0gA=="
)

var (
	// config for the test server
	testConfig, err = config.NewConfig("127.0.0.1", "1053", "foo", []string{"example.com.:../file/fixtures/example.com"}, "1m", "", "")
)

func TestInsert(t *testing.T) {
	//h := NewHandler()
	//dns.HandleFunc(fakeZone, h.serveDNS)
	//defer dns.HandleRemove(fakeZone)

	c, us, ts, addr, _ := setupTest(t, true, fakeTsigKey, fakeTsigSecret)
	defer func() { _ = us.Shutdown() }()
	defer func() { _ = ts.Shutdown() }()

	//txtRR := fmt.Sprintf("example.com. 300 IN TXT %q", "test")
	tests := []struct {
		rr       []string
		expected int
	}{
		{
			[]string{
				fmt.Sprintf("example.com. 300 IN TXT %q", "test"),
			},
			dns.RcodeSuccess,
		},
	}

	for i, tc := range tests {
		rrs := make([]dns.RR, 0)
		for _, rr := range tc.rr {
			r, err := dns.NewRR(rr)
			require.NoError(t, err, fmt.Sprintf("Failed to create RR: %s", rr))
			rrs = append(rrs, r)
		}
		m := new(dns.Msg)
		m.SetUpdate(fakeZone)
		m.SetTsig(fakeTsigKey, dns.HmacSHA256, 300, time.Now().Unix())
		m.Insert(rrs)
		r, _, err := c.Exchange(m, addr)
		require.NoError(t, err, "Failed to communicate with test server")
		assert.Equal(t, tc.expected, r.Rcode, fmt.Sprintf("Failed to exchange %d", i))
	}

}

func TestDelete(t *testing.T) {
	h := NewHandler(testConfig)
	dns.HandleFunc(fakeZone, h.serveDNS)
	defer dns.HandleRemove(fakeZone)
}

func TestExternalDNS(t *testing.T) {
	us, ts, _, tcpaddr, err := runLocalDNSTestServer(true, fakeTsigKey, fakeTsigSecret)
	require.NoError(t, err)
	defer func() { _ = us.Shutdown() }()
	defer func() { _ = ts.Shutdown() }()

	h := NewHandler(testConfig)
	dns.HandleFunc(fakeZone, h.serveDNS)
	defer dns.HandleRemove(fakeZone)

	// split addr into host and port
	_, portStr, err := net.SplitHostPort(tcpaddr)
	require.NoError(t, err)

	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)

	provider, err := rfc2136.NewRfc2136Provider(
		"127.0.0.1",
		port,
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

	// enale debug logging
	log.SetLevel(log.DebugLevel)

	recs, err := provider.Records(context.Background())
	require.NoError(t, err)
	require.Len(t, recs, 7)

	p := &plan.Changes{
		Create: []*endpoint.Endpoint{
			{
				DNSName:    "foo.example.com",
				RecordType: "A",
				Targets:    []string{"1.2.3.4"},
				RecordTTL:  endpoint.TTL(400),
			},
			{
				DNSName:    "foo.example.com",
				RecordType: "TXT",
				Targets:    []string{"boom"},
			},
			{
				DNSName:    "ns.example.com",
				RecordType: "NS",
				Targets:    []string{"boom"},
			},
		},
		Delete: []*endpoint.Endpoint{
			{
				DNSName:    "vpn.example.com",
				RecordType: "A",
				Targets:    []string{"216.146.45.240"},
			},
			{
				DNSName:    "vpn.example.com",
				RecordType: "TXT",
				Targets:    []string{"boom2"},
			},
		},
	}

	err = provider.ApplyChanges(context.Background(), p)
	require.NoError(t, err)

	recs, err = provider.Records(context.Background())
	require.NoError(t, err)
	require.Len(t, recs, 9)
}

func setupTest(t *testing.T, tsig bool, tsigKey, tsigSecret string) (*dns.Client, *dns.Server, *dns.Server, string, string) {
	us, ts, udpaddr, tcpaddr, err := runLocalDNSTestServer(tsig, tsigKey, tsigSecret)
	require.NoError(t, err, "Failed to start test server")

	c := &dns.Client{
		Net: "udp",
	}
	if tsig {
		c.TsigSecret = map[string]string{tsigKey: tsigSecret}
	}
	return c, us, ts, udpaddr, tcpaddr
}

func runLocalDNSTestServer(tsig bool, tsigKey, tsigSecret string) (*dns.Server, *dns.Server, string, string, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return nil, nil, "", "", err
	}
	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return nil, nil, "", "", err
	}
	pc, err := net.ListenPacket("udp", l.Addr().String())
	if err != nil {
		return nil, nil, "", "", err
	}

	handler := NewHandler(testConfig)
	tcpHandler := dns.NewServeMux()
	tcpHandler.HandleFunc(".", handler.serveDNS)
	udpHandler := dns.NewServeMux()
	udpHandler.HandleFunc(".", handler.serveDNS)

	udpServer := &dns.Server{
		PacketConn:   pc,
		ReadTimeout:  time.Hour,
		WriteTimeout: time.Hour,
		MsgAcceptFunc: func(dh dns.Header) dns.MsgAcceptAction {
			// bypass defaultMsgAcceptFunc to allow dynamic update (https://github.com/miekg/dns/pull/830)
			return dns.MsgAccept
		},
		Handler: udpHandler,
	}
	tcpServer := &dns.Server{
		Net:          "tcp",
		Listener:     l,
		ReadTimeout:  time.Hour,
		WriteTimeout: time.Hour,
		MsgAcceptFunc: func(dh dns.Header) dns.MsgAcceptAction {
			// bypass defaultMsgAcceptFunc to allow dynamic update (https://github.com/miekg/dns/pull/830)
			return dns.MsgAccept
		},
		Handler: tcpHandler,
	}

	if tsig {
		udpServer.TsigSecret = map[string]string{fakeTsigKey: fakeTsigSecret}
		tcpServer.TsigSecret = map[string]string{fakeTsigKey: fakeTsigSecret}
	}

	w1 := sync.Mutex{}
	w1.Lock()
	w2 := sync.Mutex{}
	w2.Lock()
	udpServer.NotifyStartedFunc = w1.Unlock
	tcpServer.NotifyStartedFunc = w2.Unlock

	fin := make(chan error, 1)
	go func() {
		fin <- udpServer.ActivateAndServe()
		pc.Close()
	}()
	go func() {
		fin <- tcpServer.ActivateAndServe()
		l.Close()
	}()

	w1.Lock()
	w2.Lock()
	return udpServer, tcpServer, pc.LocalAddr().String(), l.Addr().String(), nil
}
