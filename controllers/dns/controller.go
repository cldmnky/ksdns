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

package dns

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dnsv1alpha1 "github.com/cldmnky/ksdns/apis/dns/v1alpha1"
	rfc1035v1alpha1 "github.com/cldmnky/ksdns/pkg/zupd/api/v1alpha1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	defaultCoreDNSImage    = "quay.io/ksdns/zupd:latest"
	defaultCoreDNSReplicas = int32(2)
	kdnsVersion            = "v0.0.1"
)

// Reconciler reconciles a Ksdns object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=dns.ksdns.io,resources=ksdns,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=dns.ksdns.io,resources=ksdns/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=dns.ksdns.io,resources=ksdns/finalizers,verbs=update

// +kubebuilder:rbac:groups=rfc1035.ksdns.io,resources=zones,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rfc1035.ksdns.io,resources=zones/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rfc1035.ksdns.io,resources=zones/finalizers,verbs=update
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	/*
		There are quite a bit of intertwined resources here.
		1. The zones from the rfc1035.ksdns.io api group needs the IP of the CoreDNS service to setup correct NS records. (Use Exernal IP's here?)
		2. The CoreDNS deployment's config needs the zones from the rfc1035.ksdns.io api group to setup the Corefile. And the zupd
		   deployment (pod) ip's to setup the secondary plugin.
		3. The zupd deployment needs the zones from the rfc1035.ksdns.io api group to setup the Corefile. And the CoreDNS pod ip's to setup
		   the transfer plugin.
		4. The CoreDNS deployment and the upd deployment need the same secret to setup the Corefile for TSIG

		To solve this, we need to make sure that the secret is created first, then the Zones, then CoreDNS deployment and finally the zupd deployment.
		The secret should be created using a job that runs tsig-keygen and then creates the secret. The job should be triggered by the creation of the
		ksdns resource. The job should be deleted after the secret is created.
		Then we need to:
			1. Update the Zones with the CoreDNS service IP (zupd should reload automatically)
			2. Update the CoreDNS deployment Corefile with the zones and zupd pod IPs (CoreDNS should reload automatically)
			3. Update the zupd deployment Corefile with the zones and CoreDNS pod IPs (zupd should reload automatically)
	*/
	log := log.FromContext(ctx)
	ksdns := &dnsv1alpha1.Ksdns{}
	err := r.Get(ctx, req.NamespacedName, ksdns)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If the custom resource is not found then, it usually means that it was deleted or not created
			// In this way, we will stop the reconciliation
			log.Info("ksdns resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get ksdns")
		return ctrl.Result{}, err
	}
	// Set the status as Unknown when no status are available
	if ksdns.Status.Conditions == nil || len(ksdns.Status.Conditions) == 0 {
		meta.SetStatusCondition(&ksdns.Status.Conditions, metav1.Condition{Type: dnsv1alpha1.TypeAvailableKsdns, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})
		if err = r.Status().Update(ctx, ksdns); err != nil {
			log.Error(err, "Failed to update ksdns status")
			return ctrl.Result{}, err
		}

		if err := r.Get(ctx, req.NamespacedName, ksdns); err != nil {
			log.Error(err, "Failed to re-fetch ksdns")
			return ctrl.Result{}, err
		}
	}

	// ensureSecret
	if err := r.ensureCoreDNSSecret(ctx, ksdns); err != nil {
		log.Error(err, "Failed to ensure secret")
		return ctrl.Result{}, err
	}

	if err := r.ensureZones(ctx, ksdns); err != nil {
		log.Error(err, "Failed to ensure zones")
		return ctrl.Result{}, err
	}

	if err := r.ensureCoreDNS(ctx, ksdns); err != nil {
		log.Error(err, "Failed to ensure CoreDNS deployment")
		return ctrl.Result{}, err
	}

	if err := r.ensureZupd(ctx, ksdns); err != nil {
		log.Error(err, "Failed to ensure zupd deployment")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dnsv1alpha1.Ksdns{}).
		Owns(&rfc1035v1alpha1.Zone{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
