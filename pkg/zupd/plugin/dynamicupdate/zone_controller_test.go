package dynamicupdate

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("Memcached controller", func() {
	Context("Memcached controller test", func() {

		const MemcachedName = "test-memcached"

		ctx := context.Background()

		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      MemcachedName,
				Namespace: MemcachedName,
			},
		}

		typeNamespaceName := types.NamespacedName{Name: MemcachedName, Namespace: MemcachedName}

		BeforeEach(func() {
			By("Creating the Namespace to perform the tests")
			err := k8sClient.Create(ctx, namespace)
			Expect(err).To(Not(HaveOccurred()))

			By("Setting the Image ENV VAR which stores the Operand image")
			err = os.Setenv("MEMCACHED_IMAGE", "example.com/image:test")
			Expect(err).To(Not(HaveOccurred()))
		})

		AfterEach(func() {
			// TODO(user): Attention if you improve this code by adding other context test you MUST
			// be aware of the current delete namespace limitations. More info: https://book.kubebuilder.io/reference/envtest.html#testing-considerations
			By("Deleting the Namespace to perform the tests")
			_ = k8sClient.Delete(ctx, namespace)

			By("Removing the Image ENV VAR which stores the Operand image")
			_ = os.Unsetenv("MEMCACHED_IMAGE")
		})

		It("should successfully reconcile a custom resource for rfc1035", func() {
			By("Creating a rfc1035 resource")

			By("Reconciling the custom resource created")
			zoneReconciler := &ZoneReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			_, err := zoneReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespaceName,
			})
			Expect(err).To(Not(HaveOccurred()))
		})
	})
})
