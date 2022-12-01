package dynamicupdate

import (
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	rfc1035v1alpha1 "github.com/cldmnky/ksdns/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = log
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(rfc1035v1alpha1.AddToScheme(scheme))

}

func (d *DynamicUpdate) NewManager(cfg *rest.Config) error {
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                  scheme,
		MetricsBindAddress:      "0",
		Port:                    0,
		HealthProbeBindAddress:  "0",
		LeaderElection:          false,
		LeaderElectionID:        "3deb8c7a.ksdns.io",
		LeaderElectionNamespace: "ksdns-system",
	})
	if err != nil {
		setupLog.Error(err, "unable to setup manager")
		return err
	}
	d.Scheme = mgr.GetScheme()
	d.Client = mgr.GetClient()
	if err := d.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Zone")
		return err
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		return err
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		return err
	}
	d.mgr = mgr
	return nil
}
