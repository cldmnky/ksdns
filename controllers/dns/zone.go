package dns

import (
	"context"
	"net"

	dnsv1alpha1 "github.com/cldmnky/ksdns/apis/dns/v1alpha1"
	rfc1035v1alpha1 "github.com/cldmnky/ksdns/pkg/zupd/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *Reconciler) ensureZones(ctx context.Context, ksdns *dnsv1alpha1.Ksdns) error {
	log := log.FromContext(ctx)
	labels := makeLabels("zone", ksdns)
	for _, z := range ksdns.Spec.Zones {
		zoneSpec, err := z.ToRfc1035Zone(net.IPv4(10, 0, 0, 10))
		if err != nil {
			log.Error(err, "Failed to convert ksdns zone to rfc1035 zone")
			continue
		}
		// Create or update the zone
		zone := &rfc1035v1alpha1.Zone{
			ObjectMeta: metav1.ObjectMeta{
				Name:      z.Origin,
				Namespace: ksdns.Namespace,
				Labels:    labels,
			},
		}
		CreateOrUpdateWithRetries(ctx, r.Client, zone, func() error {
			zone.Spec = *zoneSpec
			return ctrl.SetControllerReference(ksdns, zone, r.Scheme)
		})
	}
	return nil
}
