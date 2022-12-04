package dns

import (
	"context"
	"fmt"
	"time"

	"github.com/coredns/caddy/caddyfile"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dnsv1alpha1 "github.com/cldmnky/ksdns/apis/dns/v1alpha1"
)

var _ = Describe("ksdns controller", func() {
	Context("ksdns controller test", func() {
		const ksdnsBaseName = "test-ksdns"
		var (
			typeNamespaceName = types.NamespacedName{}
			ksdnsName         string
			namespace         *corev1.Namespace
		)

		ctx := context.Background()

		BeforeEach(func() {
			By("Creating the Namespace to perform the tests")
			ksdnsName = fmt.Sprintf("%s-%d", ksdnsBaseName, time.Now().UnixMilli())
			typeNamespaceName = types.NamespacedName{Name: ksdnsBaseName, Namespace: ksdnsName}
			namespace = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ksdnsName,
					Namespace: ksdnsName,
				},
			}
			err := k8sClient.Create(ctx, namespace)
			Expect(err).To(Not(HaveOccurred()))
			By("Namespace created")
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: ksdnsName, Namespace: ksdnsName}, namespace)
			}, time.Second*10, time.Second*2).Should(Succeed())
		})

		AfterEach(func() {
			// TODO(user): Attention if you improve this code by adding other context test you MUST
			// be aware of the current delete namespace limitations. More info: https://book.kubebuilder.io/reference/envtest.html#testing-considerations
			By("Deleting the Namespace to perform the tests")
			_ = k8sClient.Delete(ctx, namespace)
		})

		It("should successfully reconcile a custom resource for ksdns", func() {
			By("Creating the custom resource for the Kind ksdns")
			ksdns := &dnsv1alpha1.Ksdns{}
			err := k8sClient.Get(ctx, typeNamespaceName, ksdns)
			if err != nil && errors.IsNotFound(err) {
				ksdns := &dnsv1alpha1.Ksdns{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ksdnsBaseName,
						Namespace: namespace.Name,
					},
					Spec: dnsv1alpha1.KsdnsSpec{
						Zones: []dnsv1alpha1.Zone{
							{
								Origin: "example.com",
								Records: []dnsv1alpha1.Record{
									{
										Name:   "www",
										Type:   "A",
										Target: "10.10.10.10",
									},
								},
							},
						},
					},
				}

				err = k8sClient.Create(ctx, ksdns)
				Expect(err).To(Not(HaveOccurred()))
			}
			By("Checking if the custom resource was successfully created")
			ksdns = &dnsv1alpha1.Ksdns{}
			Eventually(func() error {
				ksdns = &dnsv1alpha1.Ksdns{}
				return k8sClient.Get(ctx, typeNamespaceName, ksdns)
			}, time.Minute, time.Second).Should(Succeed())

			By("Reconciling the custom resource created")
			ksdnsReconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			// Reconcile the custom resource
			_, err = ksdnsReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespaceName,
			})
			Expect(err).To(Not(HaveOccurred()))

			By("Checking if the coredDNS deployment was successfully reconciled")
			coreDNSDeployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx,
					types.NamespacedName{
						Name:      fmt.Sprintf("%s-coredns", ksdns.Name),
						Namespace: ksdns.Namespace},
					coreDNSDeployment,
				)
			}, time.Minute, time.Second).Should(Succeed())

			Expect(err).To(Not(HaveOccurred()))
			Expect(coreDNSDeployment.Spec.Template.Spec.Containers[0].Image).To(Equal(defaultCoreDNSImage))

			// Reconcile the custom resource to get the configMap
			_, err = ksdnsReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespaceName,
			})
			Expect(err).To(Not(HaveOccurred()))

			By("Checking if the configMap was successfully created")
			configMap := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx,
				types.NamespacedName{
					Name:      fmt.Sprintf("%s-coredns", ksdns.Name),
					Namespace: ksdns.Namespace},
				configMap,
			)
			Expect(err).To(Not(HaveOccurred()))

		})
	})
	Context("unit tests", func() {
		Describe("setDefaults", func() {
			It("should set defaults for a ksdns resource", func() {
				ksdns := &dnsv1alpha1.Ksdns{}
				defaulted := setDefaults(ksdns)
				Expect(defaulted.Spec.CoreDNS.Image).To(Equal(defaultCoreDNSImage))
				Expect(defaulted.Spec.CoreDNS.Replicas).To(Equal(defaultCoreDNSReplicas))
			})
		})
		Describe("newCaddyFile", func() {
			It("should generate a caddyfile", func() {
				ksdnsReconciler := &Reconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}
				ksdns := &dnsv1alpha1.Ksdns{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-ksdns",
						Namespace: "test-ksdns",
					},
				}
				cf, err := ksdnsReconciler.coreDNSConfig(
					ksdns,
					[]string{"example.com", "sub.example.com"},
					false,
					[]string{"1.2.3.4", "2.3.4.5"},
				)
				Expect(err).To(Not(HaveOccurred()))
				// parse the caddyfile cf
				_, err = caddyfile.ToJSON(cf)
				Expect(err).To(Not(HaveOccurred()))
			})
		})

	})
})
