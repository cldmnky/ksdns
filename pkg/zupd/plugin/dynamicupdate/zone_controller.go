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
	"github.com/go-logr/logr"
	"github.com/miekg/dns"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	klog "sigs.k8s.io/controller-runtime/pkg/log"

	rfc1035v1alpha1 "github.com/cldmnky/ksdns/api/v1alpha1"
)

// ZoneReconciler reconciles a Zone object
type ZoneReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	log    logr.Logger
	zones  *Zones
}

// +kubebuilder:rbac:groups=rfc1035.ksdns.io,resources=zones,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rfc1035.ksdns.io,resources=zones/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rfc1035.ksdns.io,resources=zones/finalizers,verbs=update
func (r *ZoneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := klog.FromContext(ctx)
	log := clog.NewWithPlugin("dynamicupdate")
	log.Infof("Reconciling Zone %s/%s", req.Namespace, req.Name)

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

	r.zones.RLock()
	defer r.zones.RUnlock()

	if r.zones.DynamicZones == nil {
		r.zones.DynamicZones = make(map[string]*file.Zone)
	}
	if r.zones.Z == nil {
		r.zones.Z = make(map[string]*file.Zone)
	}

	if _, ok := r.zones.Z[dns.Fqdn(zone.Name)]; !ok {
		parsedZone, err := file.Parse(strings.NewReader(zone.Spec.Zone), dns.Fqdn(zone.Name), "stdin", 0)
		if err != nil {
			log.Errorf("Failed to parse zone %s: %v", zone.Name, err)
			return ctrl.Result{}, err
		}
		r.zones.Z[dns.Fqdn(zone.Name)] = parsedZone
		r.zones.DynamicZones[dns.Fqdn(zone.Name)] = file.NewZone(dns.Fqdn(zone.Name), "")
		r.zones.Names = append(r.zones.Names, dns.Fqdn(zone.Name))

	} else {
		parsedZone, err := file.Parse(strings.NewReader(zone.Spec.Zone), dns.Fqdn(zone.Name), "stdin", 0)
		if err != nil {
			log.Errorf("Failed to parse zone %s: %v", zone.Name, err)
			return ctrl.Result{}, err
		}
		r.zones.Z[dns.Fqdn(zone.Name)] = parsedZone
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ZoneReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rfc1035v1alpha1.Zone{}).
		Complete(r)
}
