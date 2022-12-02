package test

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	rfc1035v1alpha1 "github.com/cldmnky/ksdns/api/v1alpha1"
	"github.com/cldmnky/ksdns/pkg/zupd/plugin/dynamicupdate"
	"github.com/coredns/caddy"
	"github.com/miekg/dns"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider/rfc2136"
)

var (
	fakeTsigKey    string = "example.org."
	fakeTsigSecret string = "IwBTJx9wrDp4Y1RyC3H0gA=="
)

var _ = Describe("zupd", func() {
	Context("Running the binary", func() {
		var (
			caddyInstance                           *caddy.Instance
			tcp, udp                                string
			zoneName                                string = "example.org"
			subZoneName                             string = "sub.example.org"
			zupdBaseName                            string = "test-zupd"
			zupdName                                string
			namespace                               *corev1.Namespace
			typeNamespaceName, typeNamespaceSubName types.NamespacedName
		)

		ctx := context.Background()

		BeforeEach(func() {
			By("Creating the Namespace to perform the tests")
			// Create a timestamped namespace to avoid conflicts
			zupdName = fmt.Sprintf("%s-%d", zupdBaseName, time.Now().UnixMilli())
			typeNamespaceName = types.NamespacedName{Name: zoneName, Namespace: zupdName}
			typeNamespaceSubName = types.NamespacedName{Name: subZoneName, Namespace: zupdName}

			namespace = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      zupdName,
					Namespace: zupdName,
				},
			}

			err := k8sClient.Create(ctx, namespace)
			Expect(err).To(Not(HaveOccurred()))

			By("Creating new zones")
			zone := &rfc1035v1alpha1.Zone{
				ObjectMeta: metav1.ObjectMeta{
					Name:      zoneName,
					Namespace: zupdName,
				},
				Spec: rfc1035v1alpha1.ZoneSpec{
					Zone: exampleOrg,
				},
			}
			err = k8sClient.Get(ctx, typeNamespaceName, zone)
			Expect(err).To(HaveOccurred())
			err = k8sClient.Create(ctx, zone)
			Expect(err).ToNot(HaveOccurred())

			subZone := &rfc1035v1alpha1.Zone{
				ObjectMeta: metav1.ObjectMeta{
					Name:      subZoneName,
					Namespace: zupdName,
				},
				Spec: rfc1035v1alpha1.ZoneSpec{
					Zone: subExampleOrg,
				},
			}

			err = k8sClient.Get(ctx, typeNamespaceSubName, subZone)
			Expect(err).To(HaveOccurred())
			err = k8sClient.Create(ctx, subZone)
			Expect(err).ToNot(HaveOccurred())

			By("Checking that the zones are created")
			Eventually(func() error {
				found := &rfc1035v1alpha1.Zone{}
				return k8sClient.Get(ctx, typeNamespaceName, found)
			}, time.Minute, time.Second).Should(Succeed())
			Eventually(func() error {
				found := &rfc1035v1alpha1.Zone{}
				return k8sClient.Get(ctx, typeNamespaceSubName, found)
			}, time.Minute, time.Second).Should(Succeed())

			By("Adding DynamicRRs to the sub zone")
			rrs := make([]dns.RR, 0)
			rrs = append(rrs, &dns.A{
				Hdr: dns.RR_Header{
					Name:   "foo.sub.example.org.",
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    300,
				},
				A: net.ParseIP("10.10.10.10"),
			},
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   "bar.sub.example.org.",
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    300,
					},
					A: net.ParseIP("11.11.11.11"),
				},
			)
			subZone.Status.SetDynamicRRs(rrs)
			err = k8sClient.Status().Update(ctx, subZone)
			Expect(err).ToNot(HaveOccurred())
			// Check that the status is updated
			Eventually(func() error {
				found := &rfc1035v1alpha1.Zone{}
				err := k8sClient.Get(ctx, typeNamespaceSubName, found)
				if err != nil {
					return err
				}
				if len(found.Status.DynamicRRs) != 2 {
					return fmt.Errorf("expected 2 dynamic rrs, got %d", len(found.Status.DynamicRRs))
				}
				return nil
			}, time.Minute, time.Second).Should(Succeed())

			By("Starting ksdns")
			// Corefile
			corefile := `example.org:1053 {
				debug
				prometheus localhost:9253
				bind 127.0.0.1
				tsig {
					secret ` + fakeTsigKey + ` ` + fakeTsigSecret + `
					require all
				}
				dynamicupdate ` + zupdName + `
				transfer {
					to * 
					to 192.168.1.1
				}
			}`
			dynamicupdate.Cfg = cfg
			caddyInstance, udp, tcp, err = CoreDNSServerAndPorts(corefile)
			Expect(err).To(Not(HaveOccurred()))
			// ginkolog
			fmt.Fprintf(GinkgoWriter, "Started zupd, udp: %s, tcp: %s\n", udp, tcp)

		})

		AfterEach(func() {
			By("Deleting the Namespace to perform the tests")
			_ = k8sClient.Delete(ctx, namespace)
			By("Stopping ksdns")
			fmt.Fprintf(GinkgoWriter, "Running shutdown callbacks")
			errs := caddyInstance.ShutdownCallbacks()
			for _, err := range errs {
				fmt.Fprintf(GinkgoWriter, "Error during shutdown: %v", err)
			}
			caddyInstance.Stop()
		})

		Context("dynamicupdate setup", func() {
			It("Should add existing Dynamic_RR's to the zone", func() {
				// Check that the zone is updated
				Eventually(func() error {
					zone, err := dnsQuery(udp, "foo.sub.example.org.", dns.TypeA)
					if err != nil {
						return err
					}
					if len(zone.Answer) != 1 {
						return fmt.Errorf("expected 1 answers, got %d", len(zone.Answer))
					}
					return nil
				}, time.Second*6, time.Second*2).Should(Succeed())

			})
		})

		Context("dynamicupdate zones", func() {
			It("Should add and update zones", func() {
				By("Adding a new zone")
				newZone := &rfc1035v1alpha1.Zone{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "new.example.org",
						Namespace: zupdName,
					},
					Spec: rfc1035v1alpha1.ZoneSpec{
						Zone: newExampleOrg,
					},
				}
				err := k8sClient.Create(ctx, newZone)
				Expect(err).ToNot(HaveOccurred())
				By("Checking that the zone is created")
				Eventually(func() error {
					found := &rfc1035v1alpha1.Zone{}
					return k8sClient.Get(ctx, types.NamespacedName{Name: "new.example.org", Namespace: zupdName}, found)
				}, time.Minute, time.Second).Should(Succeed())
				By("Querying the new zone")
				Eventually(func() error {
					zone, err := dnsQuery(udp, "foo.new.example.org.", dns.TypeA)
					if err != nil {
						return err
					}
					if len(zone.Answer) != 1 {
						return fmt.Errorf("expected 1 answers, got %d", len(zone.Answer))
					}
					return nil
				}, time.Second*6, time.Second*2).Should(Succeed())

				By("Updating the zone")
				updatedZone := &rfc1035v1alpha1.Zone{}
				err = k8sClient.Get(ctx, types.NamespacedName{Name: "new.example.org", Namespace: zupdName}, updatedZone)
				Expect(err).ToNot(HaveOccurred())
				updatedZone.Spec.Zone = newExampleOrgUpdated
				err = k8sClient.Update(ctx, updatedZone)
				Expect(err).ToNot(HaveOccurred())

				By("Checking that the zone is updated")
				Eventually(func() error {
					zone, err := dnsQuery(udp, "bar.new.example.org.", dns.TypeA)
					if err != nil {
						return err
					}
					if len(zone.Answer) != 1 {
						return fmt.Errorf("expected 1 answers, got %d", len(zone.Answer))
					}
					return nil
				}, time.Second*30, time.Second*2).Should(Succeed())

				By("Deleting the zone")
				// get the zone again
				updatedZone = &rfc1035v1alpha1.Zone{}
				err = k8sClient.Get(ctx, types.NamespacedName{Name: "new.example.org", Namespace: zupdName}, updatedZone)
				Expect(err).ToNot(HaveOccurred())
				err = k8sClient.Delete(ctx, updatedZone)
				Expect(err).ToNot(HaveOccurred())

				By("Checking that the zone is deleted")
				Eventually(func() error {
					found := &rfc1035v1alpha1.Zone{}
					return k8sClient.Get(ctx, types.NamespacedName{Name: "new.example.org", Namespace: zupdName}, found)
				}, time.Minute, time.Second).ShouldNot(Succeed())

				By("Querying the deleted zone")
				Eventually(func() error {
					resp, err := dnsQuery(udp, "bar.new.example.org.", dns.TypeA)
					if err != nil {
						return err
					}
					if len(resp.Answer) != 0 {
						return fmt.Errorf("expected 0 answers, got %d", len(resp.Answer))
					}
					return nil
				}, time.Second*6, time.Second*2).Should(HaveOccurred())
			})
		})

		Context("external-dns", func() {
			It("should handle the external-dns rfc2136 provider", func() {
				By("Invoking external DNS")
				host, port, err := net.SplitHostPort(tcp)
				Expect(err).ToNot(HaveOccurred())
				portInt, err := strconv.Atoi(port)
				Expect(err).ToNot(HaveOccurred())
				provider, err := rfc2136.NewRfc2136Provider(
					host,
					portInt,
					zoneName+".",
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
				Expect(err).ToNot(HaveOccurred())
				recs, err := provider.Records(context.Background())
				Expect(err).ToNot(HaveOccurred())
				Expect(recs).To(HaveLen(6))

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
				Expect(err).ToNot(HaveOccurred())

				recs, err = provider.Records(context.Background())
				Expect(err).ToNot(HaveOccurred())
				Expect(recs).To(HaveLen(8))
				Eventually(func() error {
					// Check if the zone is updated
					By("Checking if the zone is updated")
					found := &rfc1035v1alpha1.Zone{}
					err = k8sClient.Get(ctx, typeNamespaceName, found)
					if err != nil {
						return err
					}
					if len(found.Status.DynamicRRs) != 2 {
						return fmt.Errorf("expected 2 records, got %d", len(found.Status.DynamicRRs))
					}
					return nil
				}, time.Minute, time.Second).Should(Succeed())
			})
		})
	})
})

func dnsQuery(addr, name string, qtype uint16) (*dns.Msg, error) {
	m := new(dns.Msg)
	dnsClient := new(dns.Client)
	dnsClient.Timeout = time.Minute
	dnsClient.Net = "tcp"
	dnsClient.TsigSecret = map[string]string{fakeTsigKey: fakeTsigSecret}
	dnsClient.SingleInflight = true
	m = m.SetTsig(fakeTsigKey, dns.HmacSHA256, 300, time.Now().Unix())

	m.SetQuestion(name, qtype)
	r, _, err := dnsClient.Exchange(m, addr)
	fmt.Fprintf(GinkgoWriter, "Message id: %v, Response id: %v", m.Id, r.Id)
	if r.Id != m.Id {
		// This is a workaround for a bug somewhere...
		return r, nil
	}
	return r, err
}
