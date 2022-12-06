package dns

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	dnsv1alpha1 "github.com/cldmnky/ksdns/apis/dns/v1alpha1"
	rfc1035v1alpha1 "github.com/cldmnky/ksdns/pkg/zupd/api/v1alpha1"
)

var (
	corednsName           = func(ksdns *dnsv1alpha1.Ksdns) string { return fmt.Sprintf("%s-coredns", ksdns.Name) }
	tsigKey               = "ksdns.tsigKey."
	coreDNSTsigSecretTmpl = template.Must(
		template.New("tsig").
			Funcs(sprig.FuncMap()).
			Parse(`key "{{.Name}}" {
  secret "{{.Secret}}";
};
`))
	coreDNSCorefileTmpl = template.Must(
		template.New("Corefile").
			Funcs(sprig.FuncMap()).
			Parse(`# This files is auto-generated by ksdns-controller. Do NOT edit.
{{range $index, $element := .Zones}}{{$element}}:1053{{end}} {
  {{if .Debug}}debug{{end}}
  {{if .Debug}}log{{end}}
  reload
  ready
  health
  prometheus
  tsig {
	secrets /etc/coredns/secret/tsig.conf
	require none
  }
  secondary {
	transfer from {{range $index, $element := .SecondaryFrom}}{{$element}}:1053 {{end}}
  }
}
`))
)

func (r *Reconciler) ensureCoreDNS(ctx context.Context, ksdns *dnsv1alpha1.Ksdns) error {
	log := log.FromContext(ctx)

	if err := r.ensureCoreDNSConfigMap(ctx, ksdns); err != nil {
		return err
	}

	if err := r.ensureCoreDNSserviceaccount(ctx, ksdns); err != nil {
		return err
	}

	deployment := coreDNSDeployment(ksdns)
	op, err := CreateOrUpdateWithRetries(ctx, r.Client, deployment, func() error {
		return ctrl.SetControllerReference(ksdns, deployment, r.Scheme)
	})
	if err != nil {
		return err
	}
	log.Info("coredns", "deployment", corednsName(ksdns), "op", op)
	return nil

}

func (r *Reconciler) ensureCoreDNSConfigMap(ctx context.Context, ksdns *dnsv1alpha1.Ksdns) error {
	log := log.FromContext(ctx)
	labels := makeLabels("coredns", ksdns)
	coreFile := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      corednsName(ksdns),
			Namespace: ksdns.Namespace,
			Labels:    labels,
		},
	}
	// see if we can get the service ip from zupd
	zupdSvc := &corev1.Service{}
	if err := r.Get(ctx, types.NamespacedName{Name: zupdName(ksdns), Namespace: ksdns.Namespace}, zupdSvc); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("zupd service not found, skipping ip lookup")
		} else {
			return err
		}
	}
	// Get the service ip from the zupd service
	// if we can't get it, we'll just use the service name
	// and hope for the best
	secondaryFrom := []string{zupdSvc.Spec.ClusterIP}
	if secondaryFrom[0] == "" {
		secondaryFrom = []string{"169.254.0.1"} // set ip to link local
	}
	// Get all zones in the current namespace
	rfc1035v1alpha1Zones := &rfc1035v1alpha1.ZoneList{}
	if err := r.List(ctx, rfc1035v1alpha1Zones, &client.ListOptions{
		Namespace: ksdns.Namespace,
	}); err != nil {
		return err
	}
	zones := []string{}
	for _, zone := range rfc1035v1alpha1Zones.Items {
		zones = append(zones, zone.Name)
	}
	// Render the corefile
	corefile, err := renderCoreDNSCorefile(zones, secondaryFrom, false)
	if err != nil {
		return err
	}
	// create or update the corefile
	op, err := CreateOrUpdateWithRetries(ctx, r.Client, coreFile, func() error {
		coreFile.Data = map[string]string{
			"Corefile": corefile,
		}
		return ctrl.SetControllerReference(ksdns, coreFile, r.Scheme)
	})
	if err != nil {
		return err
	}
	log.Info("coredns", "configmap", corednsName(ksdns), "op", op)

	return nil
}

// ensureCoreDNSSecret ensures that the secret(s) exists and has the correct data.
func (r Reconciler) ensureCoreDNSSecret(ctx context.Context, ksdns *dnsv1alpha1.Ksdns) error {
	log := log.FromContext(ctx)
	// Get the secret
	labels := makeLabels("ksdns", ksdns)
	secret := &corev1.Secret{}
	if ksdns.Spec.Secret == nil {
		// No secret specified, Create a new one if it does no exist
		var ts string

		err := r.Get(ctx, types.NamespacedName{Name: ksdns.Name, Namespace: ksdns.Namespace}, secret)
		if err != nil && apierrors.IsNotFound(err) {
			// Secret does not exist, create

			ts = generateTsigSecret()
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ksdns.Name,
					Namespace: ksdns.Namespace,
					Labels:    labels,
				},
				StringData: map[string]string{
					"tsigKey":    tsigKey,
					"tsigSecret": ts,
				},
			}
			err = ctrl.SetControllerReference(ksdns, secret, r.Scheme)
			if err != nil {
				return err
			}
			if err := r.Create(ctx, secret); err != nil {
				return err
			}
		}
	} else {
		// User provided secret, check if it has the correct data
		// try to get it
		if err := r.Get(ctx, types.NamespacedName{Name: ksdns.Spec.Secret.Name, Namespace: ksdns.Namespace}, secret); err != nil {
			log.Error(err, "failed to get user provided secret", "secret", ksdns.Spec.Secret.Name)
			return err
		}
		// check if it has the correct data
		errs := []error{}
		if _, ok := secret.Data["tsigKey"]; !ok {
			errs = append(errs, fmt.Errorf("secret %s does not have the correct data, missing tsigKey field", ksdns.Spec.Secret.Name))
		}
		if _, ok := secret.Data["tsigSecret"]; !ok {
			errs = append(errs, fmt.Errorf("secret %s does not have the correct data, missing tsigSecret field", ksdns.Spec.Secret.Name))
		}
		if len(errs) > 0 {
			return utilerrors.NewAggregate(errs)
		}
	}

	// We should now have a secret

	// Create the core DNS tsig secret
	tsigSecret, err := renderCoreFileTsigSecret(string(secret.Data["tsigKey"]), string(secret.Data["tsigSecret"]))
	if err != nil {
		return err
	}
	coreDNSSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      corednsName(ksdns),
			Namespace: ksdns.Namespace,
			Labels:    labels,
		},
	}
	op, err := CreateOrUpdateWithRetries(ctx, r.Client, coreDNSSecret, func() error {
		coreDNSSecret.StringData = map[string]string{
			"tsig.conf": tsigSecret,
		}
		return ctrl.SetControllerReference(ksdns, coreDNSSecret, r.Scheme)
	})
	if err != nil {
		return err
	}
	log.Info("coredns secret", "op", op)

	return utilerrors.NewAggregate(nil)

}

func (r *Reconciler) ensureCoreDNSserviceaccount(ctx context.Context, ksdns *dnsv1alpha1.Ksdns) error {
	log := log.FromContext(ctx)
	// Create the service account
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      corednsName(ksdns),
			Namespace: ksdns.Namespace,
			Labels:    makeLabels("ksdns", ksdns),
		},
	}
	op, err := CreateOrUpdateWithRetries(ctx, r.Client, sa, func() error {
		return ctrl.SetControllerReference(ksdns, sa, r.Scheme)
	})
	if err != nil {
		return err
	}
	log.Info("coredns", "service account", corednsName(ksdns), "op", op)
	return nil
}

func coreDNSDeployment(ksdns *dnsv1alpha1.Ksdns) *appsv1.Deployment {
	labels := makeLabels("coredns", ksdns)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      corednsName(ksdns),
			Namespace: ksdns.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &ksdns.Spec.CoreDNS.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: corednsName(ksdns),
					Containers: []corev1.Container{
						{
							Name:  "coredns",
							Image: ksdns.Spec.CoreDNS.Image,
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
							Args: []string{"run", "--conf", "/etc/coredns/config/Corefile"},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config-volume",
									MountPath: "/etc/coredns/config",
								},
								{
									Name:      "secret-volume",
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
										Name: corednsName(ksdns),
									},
								},
							},
						},
						{
							Name: "secret-volume",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: corednsName(ksdns),
								},
							},
						},
					},
				},
			},
		},
	}
	return deployment
}

func renderCoreDNSCorefile(zones, secondaryFrom []string, debug bool) (string, error) {
	params := struct {
		Zones         []string
		SecondaryFrom []string
		Debug         bool
	}{
		Zones:         zones,
		SecondaryFrom: secondaryFrom,
		Debug:         debug,
	}
	var buf bytes.Buffer
	if err := coreDNSCorefileTmpl.Execute(&buf, params); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func renderCoreFileTsigSecret(key, secret string) (string, error) {
	params := struct {
		Name   string
		Secret string
	}{
		Name:   key,
		Secret: secret,
	}
	var tmpl bytes.Buffer
	err := coreDNSTsigSecretTmpl.Execute(&tmpl, params)
	if err != nil {
		return "", err
	}
	return tmpl.String(), nil
}

func generateTsigSecret() string {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		// panic
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(key)
}
