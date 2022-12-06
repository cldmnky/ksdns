package v1alpha1

import (
	"net"
	"strings"

	"github.com/coredns/coredns/plugin/file"
	"github.com/miekg/dns"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ksdns types", func() {
	Describe("newSoaRecord", func() {
		Context("when the zone is empty", func() {
			It("should error", func() {
				_, err := newSOARecord("")
				Expect(err).To(HaveOccurred())
			})
		})
		Context("when the zone is not empty", func() {
			It("should return a SOA", func() {
				soa, err := newSOARecord("example.org")
				Expect(err).To(Not(HaveOccurred()))
				Expect(soa).To(Not(BeNil()))
				Expect(soa[0].Header().Name).To(Equal("example.org."))
				Expect(soa[0].Header().Rrtype).To(Equal(dns.TypeSOA))
			})
		})
	})

	Describe("newNSRecord", func() {
		Context("when the zone is empty", func() {
			It("should error", func() {
				_, _, err := newNSRecord("", net.ParseIP("192.168.1.1"))
				Expect(err).To(HaveOccurred())
			})
		})
		Context("when the zone is not empty", func() {
			It("should return a NS", func() {
				ns, extra, err := newNSRecord("example.org", net.ParseIP("192.168.1.1"))
				Expect(err).To(Not(HaveOccurred()))
				Expect(ns).To(Not(BeNil()))
				Expect(ns[0].Header().Name).To(Equal("example.org."))
				Expect(ns[0].Header().Rrtype).To(Equal(dns.TypeNS))
				Expect(extra).To(Not(BeNil()))
				Expect(extra[0].Header().Name).To(Equal("ns.dns.example.org."))
			})
		})
	})
	Describe("ToRfc1035Zone", func() {
		Context("when the zone is empty", func() {
			It("should error", func() {
				z := &Zone{
					Origin: "",
				}
				_, err := z.ToRfc1035Zone(net.ParseIP("192.168.1.1"))
				Expect(err).To(HaveOccurred())
			})
		})
		Context("when the zone is not empty", func() {
			It("should return a Zone", func() {
				z := &Zone{
					Origin: "example.org",
				}
				rfc1035, err := z.ToRfc1035Zone(net.ParseIP("192.168.1.1"))
				Expect(err).To(Not(HaveOccurred()))
				Expect(rfc1035.Zone).To(Not(BeNil()))
				// Parse the zone
				parsedZone, err := file.Parse(strings.NewReader(rfc1035.Zone), "example.org", "example.org", 0)
				Expect(err).To(Not(HaveOccurred()))
				Expect(parsedZone).To(Not(BeNil()))
			})
		})
		Context("when the zone has records", func() {
			It("should return a Zone with records", func() {
				z := &Zone{
					Origin: "example.org",
					Records: []Record{
						{
							Name:   "www",
							Type:   "A",
							TTL:    300,
							Target: "10.10.10.10",
						},
						// TXT record
						{
							Name: "info",
							Type: "TXT",
							TTL:  300,
							Text: "v=spf1 -all",
						},
						// CNAME record
						{
							Name:   "dns",
							Type:   "CNAME",
							TTL:    300,
							Target: "ns.dns.example.org",
						},
						// SRV record
						{
							Name:     "_sip._tcp",
							Type:     "SRV",
							TTL:      300,
							Target:   "sipserver.example.org",
							Port:     5060,
							Priority: 10,
							Weight:   20,
						},
					},
				}
				rfc1035, err := z.ToRfc1035Zone(net.ParseIP("192.168.1.1"))
				Expect(err).To(Not(HaveOccurred()))
				Expect(rfc1035.Zone).To(Not(BeNil()))
				// Parse the zone
				parsedZone, err := file.Parse(strings.NewReader(rfc1035.Zone), "example.org", "example.org", 0)
				Expect(err).To(Not(HaveOccurred()))
				Expect(parsedZone).To(Not(BeNil()))
				Expect(parsedZone.All()).To(HaveLen(5))
			})
			It("should return an error with records that are not supported", func() {
				z := &Zone{
					Origin: "example.org",
					Records: []Record{
						{
							Name:   "www",
							Type:   "MX",
							TTL:    300,
							Target: "192.168.1.1.",
						},
					},
				}
				_, err := z.ToRfc1035Zone(net.ParseIP("192.168.1.1"))
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
