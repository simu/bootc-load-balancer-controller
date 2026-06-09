package controller

import (
	"context"
	"net/netip"
	"time"

	"go.uber.org/multierr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	lb "github.com/projectsyn/bootc-load-balancer-controller/api/v1alpha1"
)

// LoadBalancerConfigReconciler reconciles a LoadBalancerConfig object
type LoadBalancerConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// General
	ConfigRoot      string
	PublicInterface string
	ClusterNetwork  netip.Prefix

	// Keepalived
	KeepalivedConfig KeepalivedConfig
}

// +kubebuilder:rbac:groups=config.bootc-lb.syn.tools,resources=loadbalancerconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=config.bootc-lb.syn.tools,resources=loadbalancerconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=config.bootc-lb.syn.tools,resources=loadbalancerconfigs/finalizers,verbs=update
// +kubebuilder:rbac:groups=v1,resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.3/pkg/reconcile
func (r *LoadBalancerConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := logf.FromContext(ctx)

	var lbconfig lb.LoadBalancerConfig
	if err := r.Get(ctx, req.NamespacedName, &lbconfig); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	l.Info("Reconciling LB config", "cloud", lbconfig.Spec.Cloud, "distribution", lbconfig.Spec.Distribution)

	errors := []error{}

	if haproxyApi, err := r.RenderHAProxyAPIConfig(ctx, &lbconfig); err == nil {
		if err := r.WriteConfig(ctx, &lbconfig.ObjectMeta, HAProxyAPIConfigFile, haproxyApi); err != nil {
			errors = append(errors, err)
		}
	} else {
		errors = append(errors, err)
	}

	if haproxyIngress, err := r.RenderHAProxyIngressConfig(ctx, &lbconfig); err == nil {
		if err := r.WriteConfig(ctx, &lbconfig.ObjectMeta, HAProxyIngressConfigFile, haproxyIngress); err != nil {
			errors = append(errors, err)
		}
	} else {
		errors = append(errors, err)
	}

	if keepalived, err := r.RenderKeepalivedConfig(ctx, &lbconfig); err == nil {
		if err := r.WriteConfig(ctx, &lbconfig.ObjectMeta, KeepalivedConfigFile, keepalived); err != nil {
			errors = append(errors, err)
		}
	} else {
		errors = append(errors, err)
	}

	if floaty, err := r.RenderFloatyConfig(ctx, &lbconfig); err == nil {
		l.Info("Floaty config", "config", floaty, "error", err)
		if err := r.WriteConfig(ctx, &lbconfig.ObjectMeta, FloatyConfigFile, floaty); err != nil {
			errors = append(errors, err)
		}
	} else {
		errors = append(errors, err)
	}

	if conntrackd, err := r.RenderConntrackdConfig(ctx, &lbconfig); err == nil {
		if err := r.WriteConfig(ctx, &lbconfig.ObjectMeta, ConntrackdConfigFile, conntrackd); err != nil {
			errors = append(errors, err)
		}
	} else {
		errors = append(errors, err)
	}

	//TODO(sg): firewall rules

	if publicNMConn, err := r.RenderPublicNMConnection(ctx, &lbconfig); err == nil {
		if err := r.WriteConfig(ctx, &lbconfig.ObjectMeta, PublicNMConnectionFile, publicNMConn); err != nil {
			errors = append(errors, err)
		}
	} else {
		errors = append(errors, err)
	}

	if clusterNetNMConn, err := r.RenderClusterNetNMConnection(ctx); err == nil {
		if err := r.WriteConfig(ctx, &lbconfig.ObjectMeta, ClusterNetworkNMConnectionFile, clusterNetNMConn); err != nil {
			errors = append(errors, err)
		}
	} else {
		errors = append(errors, err)
	}

	if keepalivedNMConn, err := r.RenderKeepalivedDummyNMConnection(ctx, &lbconfig); err == nil {
		if err := r.WriteConfig(ctx, &lbconfig.ObjectMeta, KeepalivedDummyNMConnectionFile, keepalivedNMConn); err != nil {
			errors = append(errors, err)
		}
	} else {
		errors = append(errors, err)
	}

	return ctrl.Result{}, multierr.Combine(errors...)
}

// SetupWithManager sets up the controller with the Manager.
func (r *LoadBalancerConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&lb.LoadBalancerConfig{}).
		Named("loadbalancerconfig").
		WithOptions(controller.Options{
			RateLimiter: workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](
				10*time.Second, 5*time.Minute),
		}).
		Complete(r)
}
