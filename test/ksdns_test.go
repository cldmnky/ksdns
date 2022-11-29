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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider/rfc2136"
)

var _ = Describe("zupd", func() {
	Context("zupd test", func() {
		var (
			caddyInstance  *caddy.Instance
			tcp, udp       string
			fakeTsigKey    string = "example.org."
			fakeTsigSecret string = "IwBTJx9wrDp4Y1RyC3H0gA=="
			zoneName       string = "example.org"
			err            error
		)
		const zupdName = "test-zupd"

		ctx := context.Background()

		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      zupdName,
				Namespace: zupdName,
			},
		}

		typeNamespaceName := types.NamespacedName{Name: zoneName, Namespace: zupdName}

		BeforeEach(func() {
			By("Creating the Namespace to perform the tests")
			err := k8sClient.Create(ctx, namespace)
			Expect(err).To(Not(HaveOccurred()))
			By("Starting ksdns")
			// Corefile
			corefile := `example.org:1053 {
				debug
				prometheus localhost:9253
				bind 127.0.0.1
				dynamicupdate ` + zupdName + `
				transfer {
					to * 
					to 192.168.1.1
				}
				tsig {
					secret ` + fakeTsigKey + ` ` + fakeTsigSecret + `
					require all
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

		It("should reply to external DNS", func() {

			By("Creating a new zone")
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

			By("Checking if the zone is created")
			Eventually(func() error {
				found := &rfc1035v1alpha1.Zone{}
				return k8sClient.Get(ctx, typeNamespaceName, found)
			}, time.Minute, time.Second).Should(Succeed())

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
			Expect(recs).To(HaveLen(5))

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
			Expect(recs).To(HaveLen(7))

			fmt.Fprintf(GinkgoWriter, "Running shutdown callbacks")
			caddyInstance.ShutdownCallbacks()
			fmt.Fprintf(GinkgoWriter, "Stopped caddy\n")
		})

		//It("should allow external dns", func() {
		//})
	})
})
