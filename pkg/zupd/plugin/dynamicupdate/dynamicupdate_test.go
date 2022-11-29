package dynamicupdate

import (
	"context"
	"strings"
	"time"

	rfc1035v1alpha1 "github.com/cldmnky/ksdns/api/v1alpha1"
	"github.com/coredns/coredns/plugin/file"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"
	"github.com/miekg/dns"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("zupd", func() {
	Context("Running the binary", func() {
		var (
			fakeTsigKey string = "example.org."
			//fakeTsigSecret string = "IwBTJx9wrDp4Y1RyC3H0gA=="
			zoneName string = "example.org"
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
			Cfg = cfg
			By("Creating the Namespace to perform the tests")
			err := k8sClient.Create(ctx, namespace)
			Expect(err).To(Not(HaveOccurred()))

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

		})

		AfterEach(func() {
			By("Deleting the Namespace to perform the tests")
			_ = k8sClient.Delete(ctx, namespace)
		})

		Context("Allowed Types", func() {
			It("should allow types supported", func() {
				By("Creating a dynamicupdate plugin")
				zone, err := file.Parse(strings.NewReader(exampleOrg), exampleOrgZone, "stdin", 0)
				Expect(err).ToNot(HaveOccurred())
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
				m.Insert([]dns.RR{testRR("insert.example.org 3600 IN A 127.0.0.1")})
				rec := dnstest.NewRecorder(&test.ResponseWriter{})
				code, err := d.ServeDNS(ctx, rec, m)
				Expect(err).ToNot(HaveOccurred())
				Expect(code).To(Equal(dns.RcodeSuccess))
				// Lookup the record
				m = new(dns.Msg)
				m.SetQuestion("insert.example.org.", dns.TypeA)
				rec = dnstest.NewRecorder(&test.ResponseWriter{})
				code, err = d.ServeDNS(ctx, rec, m)
				Expect(err).ToNot(HaveOccurred())
				Expect(code).To(Equal(dns.RcodeSuccess))
				Expect(rec.Msg.Answer).To(HaveLen(1))
			})
		})
	})
})

/* func TestDynamicUpdateAllowedTypes(t *testing.T) {
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

} */

func testRR(s string) dns.RR {
	r, err := dns.NewRR(s)
	Expect(err).ToNot(HaveOccurred())
	return r
}
