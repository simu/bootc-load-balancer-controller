package controller

import (
	"context"

	"go.uber.org/multierr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	corev1 "k8s.io/api/core/v1"

	lb "github.com/projectsyn/bootc-load-balancer-controller/api/v1alpha1"
)

// LoadBalancerConfigReconciler reconciles a LoadBalancerConfig object
type LoadBalancerConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// General
	ConfigRoot string

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

	var credentialSecret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: lbconfig.Spec.CloudCredentials.Name}, &credentialSecret); err != nil {
		return ctrl.Result{}, err
	}

	l.Info("Cloud credentials secret", "token", credentialSecret.Data["token"])

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

	//TODO(sg): floaty

	if conntrackd, err := r.RenderConntrackdConfig(ctx, &lbconfig); err == nil {
		if err := r.WriteConfig(ctx, &lbconfig.ObjectMeta, ConntrackdConfigFile, conntrackd); err != nil {
			errors = append(errors, err)
		}
	} else {
		errors = append(errors, err)
	}

	//TODO(sg): firewall rules

	if keepalivedNMConn, err := r.RenderKeepalivedDummyNMConnection(ctx, &lbconfig); err == nil {
		if err := r.WriteConfig(ctx, &lbconfig.ObjectMeta, KeepalivedDummyNMConnectionFile, keepalivedNMConn); err != nil {
			errors = append(errors, err)
		}
	} else {
		errors = append(errors, err)
	}

	//TODO(sg): decide who is responsible to apply internal IP to internal iface

	return ctrl.Result{}, multierr.Combine(errors...)
}

// SetupWithManager sets up the controller with the Manager.
func (r *LoadBalancerConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&lb.LoadBalancerConfig{}).
		Named("loadbalancerconfig").
		Complete(r)
}
