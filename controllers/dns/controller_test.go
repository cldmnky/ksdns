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
	rfc1035v1alpha1 "github.com/cldmnky/ksdns/pkg/zupd/api/v1alpha1"
)

var _ = Describe("ksdns controller", func() {
	Context("ksdns controller test", func() {
		const ksdnsBaseName = "test-ksdns"
		var (
			typeNamespaceName = types.NamespacedName{}
			ksdnsNameSpace    string
			namespace         *corev1.Namespace
		)

		ctx := context.Background()

		BeforeEach(func() {
			By("Creating the Namespace to perform the tests")
			ksdnsNameSpace = fmt.Sprintf("%s-%d", ksdnsBaseName, time.Now().UnixMilli())
			typeNamespaceName = types.NamespacedName{Name: ksdnsBaseName, Namespace: ksdnsNameSpace}
			namespace = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ksdnsNameSpace,
					Namespace: ksdnsNameSpace,
				},
			}
			err := k8sClient.Create(ctx, namespace)
			Expect(err).To(Not(HaveOccurred()))
			By("Namespace created")
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: ksdnsNameSpace, Namespace: ksdnsNameSpace}, namespace)
			}, time.Second*10, time.Second*2).Should(Succeed())
		})

		AfterEach(func() {
			// TODO(user): Attention if you improve this code by adding other context test you MUST
			// be aware of the current delete namespace limitations. More info: https://book.kubebuilder.io/reference/envtest.html#testing-considerations
			By("Deleting the Namespace to perform the tests")
			_ = k8sClient.Delete(ctx, namespace)
		})

		It("Should create a new secret", func() {
			By("Creating a new ksdns")
			ksdns := &dnsv1alpha1.Ksdns{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ksdnsBaseName,
					Namespace: ksdnsNameSpace,
				},
				Spec: dnsv1alpha1.KsdnsSpec{},
			}
			Expect(k8sClient.Create(ctx, ksdns)).Should(Succeed())

			By("Reconciling the custom resource created")
			ksdnsReconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			// Reconcile the custom resource
			_, err := ksdnsReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespaceName,
			})
			Expect(err).To(Not(HaveOccurred()))

			By("Checking if the secret was created")
			secret := &corev1.Secret{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: ksdnsBaseName, Namespace: ksdnsNameSpace}, secret)
			}, time.Second*10, time.Second*2).Should(Succeed())

			By("Checking if the secret has the correct data")
			Expect(secret.Data).To(HaveKey("tsigKey"))
			Expect(secret.Data).To(HaveKey("tsigSecret"))

			By("Checking That the CoreDNS secret was created")
			corednsSecret := &corev1.Secret{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: corednsName(ksdns), Namespace: ksdnsNameSpace}, corednsSecret)
			}, time.Second*10, time.Second*2).Should(Succeed())

			By("Checking if the CoreDNS secret has the correct data")
			Expect(corednsSecret.Data).To(HaveKey("tsig.conf"))
			Expect(corednsSecret.Data["tsig.conf"]).To(ContainSubstring("key \"ksdns.tsigKey.\""))

		})

		It("Should error if the user provided secret does not exist", func() {
			By("Creating a new ksdns")
			ksdns := &dnsv1alpha1.Ksdns{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ksdnsBaseName,
					Namespace: ksdnsNameSpace,
				},
				Spec: dnsv1alpha1.KsdnsSpec{
					Secret: &corev1.LocalObjectReference{
						Name: "non-existent-secret",
					},
				},
			}
			Expect(k8sClient.Create(ctx, ksdns)).Should(Succeed())

			By("Reconciling the custom resource created")
			ksdnsReconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			// Reconcile the custom resource
			_, err := ksdnsReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespaceName,
			})
			Expect(err).To(HaveOccurred())
		})

		It("Should error if the user provided secret does not have the correct fields", func() {
			By("Creating a secret")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "user-secret",
					Namespace: ksdnsNameSpace,
				},
				StringData: map[string]string{
					"test": "test",
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())

			By("Creating a new ksdns")
			ksdns := &dnsv1alpha1.Ksdns{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ksdnsBaseName,
					Namespace: ksdnsNameSpace,
				},
				Spec: dnsv1alpha1.KsdnsSpec{
					Secret: &corev1.LocalObjectReference{
						Name: "user-secret",
					},
				},
			}
			Expect(k8sClient.Create(ctx, ksdns)).Should(Succeed())

			By("Reconciling the custom resource created")
			ksdnsReconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			// Reconcile the custom resource
			_, err := ksdnsReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespaceName,
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("secret user-secret does not have the correct data, missing tsigKey field"))
			Expect(err.Error()).To(ContainSubstring("secret user-secret does not have the correct data, missing tsigSecret field"))

			By("Checking that the coreDNS secret was not created")
			corednsSecret := &corev1.Secret{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: corednsName(ksdns), Namespace: ksdnsNameSpace}, corednsSecret)
			}, time.Second*10, time.Second*2).ShouldNot(Succeed())
		})

		It("Should use the user provided secret", func() {
			var (
				tsigKey    = "test-key"
				tsigSecret = generateTsigSecret()
			)
			By("Creating a secret")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "user-secret",
					Namespace: ksdnsNameSpace,
				},
				StringData: map[string]string{
					"tsigKey":    tsigKey,
					"tsigSecret": tsigSecret,
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())

			By("Creating a new ksdns")
			ksdns := &dnsv1alpha1.Ksdns{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ksdnsBaseName,
					Namespace: ksdnsNameSpace,
				},
				Spec: dnsv1alpha1.KsdnsSpec{
					Secret: &corev1.LocalObjectReference{
						Name: "user-secret",
					},
				},
			}
			Expect(k8sClient.Create(ctx, ksdns)).Should(Succeed())

			By("Reconciling the custom resource created")
			ksdnsReconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			// Reconcile the custom resource
			_, err := ksdnsReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespaceName,
			})
			Expect(err).ToNot(HaveOccurred())

			By("Checking that the coreDNS secret was created")
			corednsSecret := &corev1.Secret{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: corednsName(ksdns), Namespace: ksdnsNameSpace}, corednsSecret)
			}, time.Second*10, time.Second*2).Should(Succeed())

			By("Checking that the coreDNS secret has the correct data")
			Expect(corednsSecret.Data["tsig.conf"]).Should(ContainSubstring("key \"test-key\""))
			Expect(corednsSecret.Data["tsig.conf"]).Should(ContainSubstring("secret"))

			By("Updating the user provided secret")
			// get the secret
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "user-secret", Namespace: ksdnsNameSpace}, secret)).Should(Succeed())
			// update the secret
			secret.Data["tsigKey"] = []byte("new-key")
			secret.Data["tsigSecret"] = []byte(generateTsigSecret())
			Expect(k8sClient.Update(ctx, secret)).Should(Succeed())

			By("Reconciling the custom resource created")
			_, err = ksdnsReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespaceName,
			})
			Expect(err).ToNot(HaveOccurred())

			By("Checking that the coreDNS secret was updated")
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: corednsName(ksdns), Namespace: ksdnsNameSpace}, corednsSecret)
			}, time.Second*10, time.Second*2).Should(Succeed())
			Expect(corednsSecret.Data["tsig.conf"]).Should(ContainSubstring("key \"new-key\""))
			Expect(corednsSecret.Data["tsig.conf"]).ShouldNot(ContainSubstring(tsigKey))

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

			By("Checking that the zone was created")
			zone := &rfc1035v1alpha1.Zone{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: ksdns.Spec.Zones[0].Origin, Namespace: ksdnsNameSpace}, zone)
			}, time.Second*10, time.Second*2).Should(Succeed())

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

			By("Checking that the CoreDNS deployment have 2 pods running")
			Eventually(func() (int32, error) {
				coreDNSDeployment := &appsv1.Deployment{}
				err := k8sClient.Get(ctx,
					types.NamespacedName{
						Name:      fmt.Sprintf("%s-coredns", ksdns.Name),
						Namespace: ksdns.Namespace},
					coreDNSDeployment,
				)
				return coreDNSDeployment.Status.ReadyReplicas, err
			}, time.Minute, time.Second).Should(Equal(int32(2)))

			By("Checking that the zupd deployment have 2 updated replicas")
			zupdDeployment := &appsv1.Deployment{}
			Eventually(func() (int32, error) {
				zupdDeployment := &appsv1.Deployment{}
				err := k8sClient.Get(ctx,
					types.NamespacedName{
						Name:      fmt.Sprintf("%s-zupd", ksdns.Name),
						Namespace: ksdns.Namespace},
					zupdDeployment,
				)
				return zupdDeployment.Status.UpdatedReplicas, err
			}, time.Minute, time.Second).Should(Equal(int32(2)))
			Expect(zupdDeployment.Status.ReadyReplicas).To(Equal(int32(1)))
			Eventually(func() (int32, error) {
				zupdDeployment := &appsv1.Deployment{}
				err := k8sClient.Get(ctx,
					types.NamespacedName{
						Name:      fmt.Sprintf("%s-zupd", ksdns.Name),
						Namespace: ksdns.Namespace},
					zupdDeployment,
				)
				return zupdDeployment.Status.ReadyReplicas, err
			}).Should(Equal(int32(1)))
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
				cf, err := renderCoreDNSCorefile(
					[]string{"example.com", "sub.example.com"},
					[]string{"1.2.3.4", "2.3.4.5"},
					false,
				)
				Expect(err).To(Not(HaveOccurred()))
				// parse the caddyfile cf
				_, err = caddyfile.ToJSON([]byte(cf))
				Expect(err).To(Not(HaveOccurred()))
			})
		})

	})
})
