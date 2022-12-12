package dns

import (
	"bytes"
	"context"
	"fmt"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	dnsv1alpha1 "github.com/cldmnky/ksdns/apis/dns/v1alpha1"
	rfc1035v1alpha1 "github.com/cldmnky/ksdns/pkg/zupd/api/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	zupdName         = func(ksdns *dnsv1alpha1.Ksdns) string { return fmt.Sprintf("%s-zupd", ksdns.Name) }
	zupdCorefileTmpl = template.Must(
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
  dynamicupdate {{.Namespace}}
  transfer {
    to *
    #{{range $index, $element := .TransferTo}}to {{$element}}
    #{{end}}
  }
}
`))
)

func (r *Reconciler) ensureZupd(ctx context.Context, ksdns *dnsv1alpha1.Ksdns) error {
	log := log.FromContext(ctx)

	if err := r.ensureZupdSvc(ctx, ksdns); err != nil {
		return err
	}

	if err := r.ensureZupdServiceAccount(ctx, ksdns); err != nil {
		return err
	}
	if err := r.ensureZupdNamespaceAdminRole(ctx, ksdns); err != nil {
		return err
	}
	if err := r.ensureZupdConfigMap(ctx, ksdns); err != nil {
		return err
	}

	deployment := zupdDeployment(ksdns)
	op, err := CreateOrUpdateWithRetries(ctx, r.Client, deployment, func() error {
		return ctrl.SetControllerReference(ksdns, deployment, r.Scheme)
	})
	if err != nil {
		return err
	}
	log.Info("zupd", "deployment", zupdName(ksdns), "op", op)
	return nil
}

func (r *Reconciler) ensureZupdServiceAccount(ctx context.Context, ksdns *dnsv1alpha1.Ksdns) error {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      zupdName(ksdns),
			Namespace: ksdns.Namespace,
		},
	}
	_, err := CreateOrUpdateWithRetries(ctx, r.Client, sa, func() error {
		return ctrl.SetControllerReference(ksdns, sa, r.Scheme)
	})
	if err != nil {
		return err
	}
	return nil
}

func (r *Reconciler) ensureZupdNamespaceAdminRole(ctx context.Context, ksdns *dnsv1alpha1.Ksdns) error {
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      zupdName(ksdns),
			Namespace: ksdns.Namespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"rfc1035.ksdns.io"},
				Resources: []string{"zones"},
				Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
			},
			// leases
			{
				APIGroups: []string{"coordination.k8s.io"},
				Resources: []string{"leases"},
				Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
			},
			// Events
			{
				APIGroups: []string{""},
				Resources: []string{"events"},
				Verbs:     []string{"create", "patch"},
			},
		},
	}
	op, err := CreateOrUpdateWithRetries(ctx, r.Client, role, func() error {
		return ctrl.SetControllerReference(ksdns, role, r.Scheme)
	})
	if err != nil {
		return err
	}
	log.FromContext(ctx).Info("zupd namespace admin role", "op", op)
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      zupdName(ksdns),
			Namespace: ksdns.Namespace,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      zupdName(ksdns),
				Namespace: ksdns.Namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "Role",
			Name:     zupdName(ksdns),
			APIGroup: "rbac.authorization.k8s.io",
		},
	}
	op, err = CreateOrUpdateWithRetries(ctx, r.Client, roleBinding, func() error {
		return ctrl.SetControllerReference(ksdns, roleBinding, r.Scheme)
	})
	if err != nil {
		return err
	}
	log.FromContext(ctx).Info("zupd namespace admin role binding", "op", op)
	return nil

}

func (r *Reconciler) ensureZupdSvc(ctx context.Context, ksdns *dnsv1alpha1.Ksdns) error {
	labels := makeLabels("zupd", ksdns)
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      zupdName(ksdns),
			Namespace: ksdns.Namespace,
			Labels:    labels,
		},
	}
	CreateOrUpdateWithRetries(ctx, r.Client, svc, func() error {
		svc.Spec = corev1.ServiceSpec{
			Selector: makeSelector("zupd", ksdns),
			Ports: []corev1.ServicePort{
				{
					Name:       "dns-tcp",
					Port:       1053,
					TargetPort: intstr.FromInt(1053),
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "dns-udp",
					Port:       1053,
					TargetPort: intstr.FromInt(1053),
					Protocol:   corev1.ProtocolUDP,
				},
			},
		}
		return ctrl.SetControllerReference(ksdns, svc, r.Scheme)
	})
	return nil
	// see if we can get the service ip from zupd
}

func (r *Reconciler) ensureZupdConfigMap(ctx context.Context, ksdns *dnsv1alpha1.Ksdns) error {
	log := log.FromContext(ctx)
	labels := makeLabels("zupd", ksdns)
	coreFile := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      zupdName(ksdns),
			Namespace: ksdns.Namespace,
			Labels:    labels,
		},
	}
	// see if we can get the pod's ip from coredns
	coreDNSPods := &corev1.PodList{}
	if err := r.Client.List(ctx, coreDNSPods, client.InNamespace(ksdns.Namespace), client.MatchingLabels(makeLabels("coredns", ksdns))); err != nil {
		return err
	}
	transferTo := []string{}
	for _, pod := range coreDNSPods.Items {
		if pod.Status.PodIP != "" {
			transferTo = append(transferTo, pod.Status.PodIP)
			log.Info("found coredns pod", "pod", pod.Name, "ip", pod.Status.PodIP)
		}
	}
	if len(transferTo) == 0 {
		transferTo = append(transferTo, "169.254.0.1")
		log.Info("no coredns pods found, using default", "ip", "169.254.0.1")
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
	corefile, err := renderZupdDNSCorefile(zones, transferTo, ksdns.Namespace, false)
	if err != nil {
		return err
	}
	// create or update the corefile
	CreateOrUpdateWithRetries(ctx, r.Client, coreFile, func() error {
		coreFile.Data = map[string]string{
			"Corefile": corefile,
		}
		return ctrl.SetControllerReference(ksdns, coreFile, r.Scheme)
	})

	return nil
}

func renderZupdDNSCorefile(zones, transferTo []string, namespace string, debug bool) (string, error) {
	params := struct {
		Zones      []string
		TransferTo []string
		Namespace  string
		Debug      bool
	}{
		Zones:      zones,
		TransferTo: transferTo,
		Debug:      debug,
		Namespace:  namespace,
	}
	var buf bytes.Buffer
	if err := zupdCorefileTmpl.Execute(&buf, params); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func zupdDeployment(ksdns *dnsv1alpha1.Ksdns) *appsv1.Deployment {
	labels := makeLabels("zupd", ksdns)
	replicas := int32(2)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      zupdName(ksdns),
			Namespace: ksdns.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: makeSelector("zupd", ksdns),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: zupdName(ksdns),
					Containers: []corev1.Container{
						{
							Name:  "zupd",
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
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/ready",
										Port: intstr.FromInt(8181),
									},
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt(8080),
									},
								},
							},
							Args: []string{
								"run",
								"--conf",
								"/etc/coredns/config/Corefile",
								"--enable-leader-election",
								"--leader-election-namespace",
								ksdns.Namespace,
							},
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
										Name: zupdName(ksdns),
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
