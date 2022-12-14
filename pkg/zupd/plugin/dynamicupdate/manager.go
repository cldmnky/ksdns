package dynamicupdate

import (
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	rfc1035v1alpha1 "github.com/cldmnky/ksdns/pkg/zupd/api/v1alpha1"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	scheme                  = runtime.NewScheme()
	setupLog                = log
	enableLeaderElection    bool
	leaderElectionNamespace string
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(rfc1035v1alpha1.AddToScheme(scheme))

}

func (d *DynamicUpdate) NewManager(cfg *rest.Config) error {
	v := viper.GetViper()
	election := v.Get("enable-leader-election")
	if election == nil {
		enableLeaderElection = false
	} else {
		enableLeaderElection = election.(bool)
	}
	electionNamespace := v.Get("leader-election-namespace")
	if electionNamespace == nil {
		leaderElectionNamespace = ""
	} else {
		leaderElectionNamespace = electionNamespace.(string)
	}

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                  scheme,
		NewCache:                cache.MultiNamespacedCacheBuilder(d.Namespaces),
		MetricsBindAddress:      "0",
		Port:                    0,
		HealthProbeBindAddress:  "0",
		LeaderElection:          enableLeaderElection,
		LeaderElectionID:        "3deb8c7a.ksdns.io",
		LeaderElectionNamespace: leaderElectionNamespace,
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
