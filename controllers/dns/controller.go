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
	"bytes"
	"context"
	"fmt"
	"net"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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
	defaultCoreDNSImage    = "coredns/coredns:1.10.0"
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
	// ensureZones
	// ensureCoreDNS
	// ensureZupd

	// Create the Zone CR
	//zone := &rfc1035v1alpha1.Zone{}
	for _, z := range ksdns.Spec.Zones {
		zoneSpec, err := z.ToRfc1035Zone(net.IPv4(10, 0, 0, 10))
		if err != nil {
			log.Error(err, "Failed to convert ksdns zone to rfc1035 zone")
			continue
		}
		zoneSpec.Namespace = ksdns.Namespace
		zoneSpec.Labels = makeLabels("zone", ksdns)
		if err := ctrl.SetControllerReference(ksdns, zoneSpec, r.Scheme); err != nil {
			log.Error(err, "Failed to set owner reference on zone")
			return ctrl.Result{}, err
		}
		if err := r.Create(ctx, zoneSpec); err != nil {
			if !apierrors.IsAlreadyExists(err) {
				log.Error(err, "Failed to create zone")
				return ctrl.Result{}, err
			}
		}
	}

	// Create a deployment for ksdns and one for CoreDNS
	coreDNSName := types.NamespacedName{Name: fmt.Sprintf("%s-coredns", ksdns.Name), Namespace: ksdns.Namespace}
	coreDNSDeployment := &appsv1.Deployment{}

	// Try to get the deployment, if it doesn't exist then create it
	err = r.Get(ctx, coreDNSName, coreDNSDeployment)
	if err != nil && apierrors.IsNotFound(err) {
		// Define a new deployment
		coreDNSDeployment = r.coreDNSDeployment(ksdns)
		err = r.Create(ctx, coreDNSDeployment)
		if err != nil {
			log.Error(err, "Failed to create new Deployment", "Deployment.Namespace", coreDNSDeployment.Namespace, "Deployment.Name", coreDNSDeployment.Name)
			return ctrl.Result{}, err
		}
		// Deployment created successfully - return and requeue
		return ctrl.Result{Requeue: true}, nil
	}

	// Try go get the CoreDNS config
	coreDNSConfig := &corev1.ConfigMap{}
	coreDNSConfigName := types.NamespacedName{Name: fmt.Sprintf("%s-coredns", ksdns.Name), Namespace: ksdns.Namespace}
	err = r.Get(ctx, coreDNSConfigName, coreDNSConfig)
	if err != nil && apierrors.IsNotFound(err) {
		// Define a new configmap
		coreDNSConfig, err = r.coreDNSConfigMap(ksdns)
		if err != nil {
			log.Error(err, "Failed to create new ConfigMap", "ConfigMap.Namespace", coreDNSConfig.Namespace, "ConfigMap.Name", coreDNSConfig.Name)
			return ctrl.Result{}, err
		}
		err = r.Create(ctx, coreDNSConfig)
		if err != nil {
			log.Error(err, "Failed to create new ConfigMap", "ConfigMap.Namespace", coreDNSConfig.Namespace, "ConfigMap.Name", coreDNSConfig.Name)
			return ctrl.Result{}, err
		}
		// ConfigMap created successfully - return and requeue
		return ctrl.Result{Requeue: true}, nil
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

func (r *Reconciler) coreDNSSecret(ksdns *dnsv1alpha1.Ksdns) *corev1.Secret {
	// generate a key using dnssec-key gen
	// secret ref in ksdns
	labels := makeLabels("coredns", ksdns)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-coredns", ksdns.Name),
			Namespace: ksdns.Namespace,
			Labels:    labels,
		},
		Data: map[string][]byte{
			"Corefile": []byte(`.:53`),
		},
	}
	ctrl.SetControllerReference(ksdns, secret, r.Scheme)
	return secret
}

func (r *Reconciler) coreDNSConfigMap(ksdns *dnsv1alpha1.Ksdns) (*corev1.ConfigMap, error) {
	// Get a list of zones we need to add to the CoreDNS config
	z := &rfc1035v1alpha1.ZoneList{}
	err := r.List(context.Background(), z, client.InNamespace(ksdns.Namespace))
	if err != nil {
		return nil, err
	}
	zones := []string{}
	for _, zone := range z.Items {
		// Add the zone to the CoreDNS config
		zones = append(zones, zone.Name)
	}

	// TODO get the zupd pod ip
	// Get the CoreDNS config
	coreDNSConfig, err := r.coreDNSConfig(ksdns, zones, false, []string{"10.10.10.10"})
	if err != nil {
		return nil, err
	}
	labels := makeLabels("coredns", ksdns)
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-coredns", ksdns.Name),
			Namespace: ksdns.Namespace,
			Labels:    labels,
		},
		Data: map[string]string{
			"Corefile": string(coreDNSConfig),
		},
	}
	err = ctrl.SetControllerReference(ksdns, configMap, r.Scheme)
	if err != nil {
		return nil, err
	}
	return configMap, nil
}

// make labels for the deployment
func makeLabels(name string, ksdns *dnsv1alpha1.Ksdns) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       name,
		"app.kubernetes.io/version":    kdnsVersion,
		"app.kubernetes.io/managed-by": "ksdns",
		"app.kubernetes.io/instance":   ksdns.Name,
	}
}

func (r *Reconciler) coreDNSDeployment(ksdns *dnsv1alpha1.Ksdns) *appsv1.Deployment {
	defaultedKsdns := setDefaults(ksdns)
	labels := makeLabels("coredns", defaultedKsdns)
	replicas := defaultedKsdns.Spec.CoreDNS.Replicas
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-coredns", ksdns.Name),
			Namespace: ksdns.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "coredns",
							Image: defaultedKsdns.Spec.CoreDNS.Image,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 1053,
									Protocol:      corev1.ProtocolTCP,
								},
								{
									ContainerPort: 1053,
									Protocol:      corev1.ProtocolUDP,
								},
							},
							Args: []string{"-conf", "/etc/coredns/config/Corefile"},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config-volume",
									MountPath: "/etc/coredns/config",
								},
								{
									Name:      "config-secret",
									MountPath: "/etc/coredns/secret",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config-volume",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: fmt.Sprintf("%s-coredns", ksdns.Name),
									},
								},
							},
						},
						{
							Name: "config-secret",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: fmt.Sprintf("%s-coredns", ksdns.Name),
								},
							},
						},
					},
				},
			},
		},
	}
	err := ctrl.SetControllerReference(ksdns, deployment, r.Scheme)
	if err != nil {
		return nil
	}
	return deployment
}

func setDefaults(ksdns *dnsv1alpha1.Ksdns) *dnsv1alpha1.Ksdns {
	defaultedKsdns := ksdns.DeepCopy()
	if defaultedKsdns.Spec.CoreDNS.Image == "" {
		defaultedKsdns.Spec.CoreDNS.Image = defaultCoreDNSImage
	}
	if defaultedKsdns.Spec.CoreDNS.Replicas == 0 {
		defaultedKsdns.Spec.CoreDNS.Replicas = defaultCoreDNSReplicas
	}
	return defaultedKsdns
}

func (r *Reconciler) zupdConfig(ksdns *dnsv1alpha1.Ksdns, zones []string, debug bool, transferTo []string) ([]byte, error) {
	conf := `# This files is auto-generated by ksdns-controller. Do NOT edit.
{{range $index, $element := .Zones}}{{$element}}:1053 {{end}} {
  {{if .Debug}}debug{{end}}
  {{if .Debug}}log{{end}}
  reload
  ready
  health
  prometheus
  dynamicupdate {{.Namespace}}
  transfer {
    to *
    {{range $index, $element := .TransferTo}}to {{$element}}
    {{end}}
  }
  tsig {
    secrets /etc/coredns/secret/tsig.conf
    require all
  }
}
		`
	tmpl, err := template.New("caddyfile").Funcs(sprig.FuncMap()).Parse(conf)
	if err != nil {
		return nil, err
	}
	var caddyfile bytes.Buffer
	err = tmpl.Execute(&caddyfile, struct {
		Zones      []string
		Debug      bool
		Namespace  string
		TransferTo []string
	}{
		Zones:      zones,
		Debug:      debug,
		Namespace:  ksdns.Namespace,
		TransferTo: transferTo,
	})
	if err != nil {
		return nil, err
	}
	return caddyfile.Bytes(), nil
}

func (r *Reconciler) coreDNSConfig(ksdns *dnsv1alpha1.Ksdns, zones []string, debug bool, secondaryFrom []string) ([]byte, error) {
	conf := `# This files is auto-generated by ksdns-controller. Do NOT edit.
{{range $index, $element := .Zones}}{{$element}}:1053{{end}} {
  {{if .Debug}}debug{{end}}
  {{if .Debug}}log{{end}}
  reload
  ready
  health
  prometheus
  secondary {
	transfer from {{range $index, $element := .SecondaryFrom}}{{$element}}:1053 {{end}}
  }
  tsig {
    secrets /etc/coredns/secret/tsig.conf
    require all
  }
}
		`
	tmpl, err := template.New("caddyfile").Funcs(sprig.FuncMap()).Parse(conf)
	if err != nil {
		return nil, err
	}
	var caddyfile bytes.Buffer
	err = tmpl.Execute(&caddyfile, struct {
		Zones         []string
		Debug         bool
		Namespace     string
		SecondaryFrom []string
	}{
		Zones:         zones,
		Debug:         debug,
		Namespace:     ksdns.Namespace,
		SecondaryFrom: secondaryFrom,
	})
	if err != nil {
		return nil, err
	}
	return caddyfile.Bytes(), nil
}
