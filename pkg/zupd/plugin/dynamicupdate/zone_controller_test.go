package dynamicupdate

import (
	"context"
	"time"

	rfc1035v1alpha1 "github.com/cldmnky/ksdns/api/v1alpha1"
	"github.com/miekg/dns"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("zupd controller", func() {
	Context("zupd controller test", func() {

		var (
			zoneName  string = "example.org"
			zupdName  string = "test-zupd"
			namespace *corev1.Namespace
		)

		ctx := context.Background()

		namespace = &corev1.Namespace{
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
		})

		AfterEach(func() {
			By("Deleting the Namespace to perform the tests")
			_ = k8sClient.Delete(ctx, namespace)
		})

		It("should successfully reconcile a custom resource for rfc1035", func() {
			By("Creating a rfc1035 resource")

			zone := &rfc1035v1alpha1.Zone{
				ObjectMeta: metav1.ObjectMeta{
					Name:      zoneName,
					Namespace: zupdName,
				},
				Spec: rfc1035v1alpha1.ZoneSpec{
					Zone: exampleOrg,
				},
			}
			err := k8sClient.Get(ctx, typeNamespaceName, zone)
			Expect(err).To(HaveOccurred())
			err = k8sClient.Create(ctx, zone)
			Expect(err).ToNot(HaveOccurred())

			By("Checking if the zone is created")
			Eventually(func() error {
				found := &rfc1035v1alpha1.Zone{}
				return k8sClient.Get(ctx, typeNamespaceName, found)
			}, time.Second*20, time.Second).Should(Succeed())

			By("Reconciling the custom resource created")
			zones := &Zones{}
			zoneReconciler := &ZoneReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				zones:  zones,
			}
			_, err = zoneReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespaceName,
			})
			Expect(err).To(Not(HaveOccurred()))

			By("Checking if the zone is updated")
			zoneReconciler.zones.RLock()
			Expect(zoneReconciler.zones.Z).To(HaveLen(1))
			Expect(zoneReconciler.zones.Z[dns.Fqdn(zoneName)]).ToNot(BeNil())
			Expect(zoneReconciler.zones.Names).To(HaveLen(1))
			Expect(zoneReconciler.zones.Names).To(ContainElement(dns.Fqdn(zoneName)))
			Expect(zoneReconciler.zones.Z[dns.Fqdn(zoneName)].All()).To(HaveLen(5))
			zoneReconciler.zones.RUnlock()

			By("Updating the zone")
			zone.Spec.Zone = exampleOrgUpdated
			err = k8sClient.Update(ctx, zone)
			Expect(err).ToNot(HaveOccurred())

			By("Reconciling the custom resource updated")
			_, err = zoneReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespaceName,
			})
			Expect(err).To(Not(HaveOccurred()))

			By("Checking if the zone is updated")
			zoneReconciler.zones.RLock()
			Expect(zoneReconciler.zones.Z).To(HaveLen(1))
			Expect(zoneReconciler.zones.Z[dns.Fqdn(zoneName)]).ToNot(BeNil())
			Expect(zoneReconciler.zones.Names).To(HaveLen(1))
			Expect(zoneReconciler.zones.Names).To(ContainElement(dns.Fqdn(zoneName)))
			Expect(zoneReconciler.zones.Z[dns.Fqdn(zoneName)].All()).To(HaveLen(1))
			zoneReconciler.zones.RUnlock()

		})
	})
})
