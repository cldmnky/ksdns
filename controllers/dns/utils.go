package dns

import (
	"context"

	dnsv1alpha1 "github.com/cldmnky/ksdns/apis/dns/v1alpha1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// make labels for the deployment
func makeLabels(name string, ksdns *dnsv1alpha1.Ksdns) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       name,
		"app.kubernetes.io/version":    kdnsVersion,
		"app.kubernetes.io/managed-by": "ksdns",
		"app.kubernetes.io/instance":   ksdns.Name,
	}
}

// make selector for the deployment
func makeSelector(name string, ksdns *dnsv1alpha1.Ksdns) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":     name,
		"app.kubernetes.io/instance": ksdns.Name,
	}
}

// CreateOrUpdateWithRetries creates or updates the given object in the Kubernetes with retries
func CreateOrUpdateWithRetries(
	ctx context.Context,
	c client.Client,
	obj client.Object,
	f controllerutil.MutateFn,
) (controllerutil.OperationResult, error) {
	var operationResult controllerutil.OperationResult
	updateErr := wait.ExponentialBackoff(retry.DefaultBackoff, func() (ok bool, err error) {
		operationResult, err = controllerutil.CreateOrUpdate(ctx, c, obj, f)
		if err == nil {
			return true, nil
		}
		if !apierrors.IsConflict(err) {
			return false, err
		}
		return false, nil
	})
	return operationResult, updateErr
}
