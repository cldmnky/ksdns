/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package dynamicupdate

import (
	"context"
	"strings"

	"github.com/coredns/coredns/plugin/file"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/miekg/dns"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	klog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	rfc1035v1alpha1 "github.com/cldmnky/ksdns/api/v1alpha1"
)

// +kubebuilder:rbac:groups=rfc1035.ksdns.io,resources=zones,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rfc1035.ksdns.io,resources=zones/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rfc1035.ksdns.io,resources=zones/finalizers,verbs=update
func (r *DynamicUpdate) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := klog.FromContext(ctx)
	log := clog.NewWithPlugin("dynamicupdate")

	zone := &rfc1035v1alpha1.Zone{}
	err := r.Get(ctx, req.NamespacedName, zone)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If the custom resource is not found then, it usually means that it was deleted or not created
			// In this way, we will stop the reconciliation
			logger.Info("zone resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "Failed to get zone")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if isDeleting(zone) {
		log.Debugf("Zone %s/%s is being deleted", req.Namespace, req.Name)
		if _, ok := r.Zones.Z[dns.Fqdn(zone.Name)]; ok {
			log.Debugf("Zone %s/%s is being deleted", req.Namespace, req.Name)
			r.Zones.DeleteZone(dns.Fqdn(zone.Name))
			return ctrl.Result{}, nil
		}
	}

	if r.Zones.DynamicZones == nil {
		r.Zones.DynamicZones = make(map[string]*file.Zone)
	}
	if r.Zones.Z == nil {
		r.Zones.Z = make(map[string]*file.Zone)
	}
	r.Zones.Lock()
	defer r.Zones.Unlock()
	if _, ok := r.Zones.Z[dns.Fqdn(zone.Name)]; !ok {
		// Create a new zone
		log.Debugf("Creating new zone %s", zone.Name)
		parsedZone, err := file.Parse(strings.NewReader(zone.Spec.Zone), dns.Fqdn(zone.Name), "stdin", 0)
		if err != nil {
			log.Errorf("Failed to parse zone %s: %v", zone.Name, err)
			return ctrl.Result{}, err
		}
		r.Zones.Z[dns.Fqdn(zone.Name)] = parsedZone
		r.Zones.DynamicZones[dns.Fqdn(zone.Name)] = file.NewZone(dns.Fqdn(zone.Name), "")
		r.Zones.Names = append(r.Zones.Names, dns.Fqdn(zone.Name))
		r.transfer.Notify(dns.Fqdn(zone.Name))

	} else {
		// Update the zone if it has changed, compare old and new object
		log.Debugf("Zone %s has changed", zone.Name)
		parsedZone, err := file.Parse(strings.NewReader(zone.Spec.Zone), dns.Fqdn(zone.Name), "stdin", 0)
		if err != nil {
			log.Errorf("Failed to parse zone %s: %v", zone.Name, err)
			return ctrl.Result{}, err
		}
		r.Zones.Z[dns.Fqdn(zone.Name)] = parsedZone
		r.transfer.Notify(dns.Fqdn(zone.Name))

	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DynamicUpdate) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rfc1035v1alpha1.Zone{}).
		// Ignore status-only changes
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}

func isDeleting(zone *rfc1035v1alpha1.Zone) bool {
	return !zone.ObjectMeta.GetDeletionTimestamp().IsZero()
}
