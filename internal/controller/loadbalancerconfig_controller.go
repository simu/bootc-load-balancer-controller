package controller

import (
	"context"
	"fmt"
	"net/netip"
	"time"

	"go.uber.org/multierr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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

type LocalBackendConfiguration struct {
	API     []Backend
	Ingress []Backend
}

// +kubebuilder:rbac:groups=config.bootc-lb.syn.tools,resources=loadbalancerconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=config.bootc-lb.syn.tools,resources=loadbalancerconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=config.bootc-lb.syn.tools,resources=loadbalancerconfigs/finalizers,verbs=update
// +kubebuilder:rbac:groups=v1,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=v1,resources=nodes,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.3/pkg/reconcile
func (r *LoadBalancerConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var lbconfig lb.LoadBalancerConfig
	if err := r.Get(ctx, req.NamespacedName, &lbconfig); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var credentialSecret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: lbconfig.Namespace,
		Name:      lbconfig.Spec.CloudCredentials.Name,
	}, &credentialSecret); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to read cloud provider credentials: %w", err)
	}

	return r.ReconcileLBConfig(ctx, &lbconfig, &credentialSecret, nil)
}

func (r *LoadBalancerConfigReconciler) ReconcileLBConfig(ctx context.Context, lbconfig *lb.LoadBalancerConfig, credentialSecret *corev1.Secret, localBackends *LocalBackendConfiguration) (ctrl.Result, error) {
	l := logf.FromContext(ctx)

	l.Info("Reconciling LB config", "cloud", lbconfig.Spec.Cloud, "distribution", lbconfig.Spec.Distribution)

	errors := []error{}

	if haproxyApi, err := r.RenderHAProxyAPIConfig(ctx, lbconfig, localBackends); err == nil {
		if err := r.WriteConfig(ctx, &lbconfig.ObjectMeta, HAProxyAPIConfigFile, haproxyApi, 0644); err != nil {
			errors = append(errors, err)
		}
	} else {
		errors = append(errors, err)
	}

	if haproxyIngress, err := r.RenderHAProxyIngressConfig(ctx, lbconfig, localBackends); err == nil {
		if err := r.WriteConfig(ctx, &lbconfig.ObjectMeta, HAProxyIngressConfigFile, haproxyIngress, 0644); err != nil {
			errors = append(errors, err)
		}
	} else {
		errors = append(errors, err)
	}

	if keepalived, err := r.RenderKeepalivedConfig(ctx, lbconfig); err == nil {
		if err := r.WriteConfig(ctx, &lbconfig.ObjectMeta, KeepalivedConfigFile, keepalived, 0644); err != nil {
			errors = append(errors, err)
		}
	} else {
		errors = append(errors, err)
	}

	if floaty, err := r.RenderFloatyConfig(ctx, lbconfig, credentialSecret); err == nil {
		l.Info("Floaty config", "config", floaty, "error", err)
		if err := r.WriteConfig(ctx, &lbconfig.ObjectMeta, FloatyConfigFile, floaty, 0644); err != nil {
			errors = append(errors, err)
		}
	} else {
		errors = append(errors, err)
	}

	if conntrackd, err := r.RenderConntrackdConfig(ctx, lbconfig); err == nil {
		if err := r.WriteConfig(ctx, &lbconfig.ObjectMeta, ConntrackdConfigFile, conntrackd, 0644); err != nil {
			errors = append(errors, err)
		}
	} else {
		errors = append(errors, err)
	}

	if publicNMConn, err := r.RenderPublicNMConnection(ctx, lbconfig); err == nil {
		if err := r.WriteConfig(ctx, &lbconfig.ObjectMeta, PublicNMConnectionFile, publicNMConn, 0600); err != nil {
			errors = append(errors, err)
		}
	} else {
		errors = append(errors, err)
	}

	if clusterNetNMConn, err := r.RenderClusterNetNMConnection(ctx); err == nil {
		if err := r.WriteConfig(ctx, &lbconfig.ObjectMeta, ClusterNetworkNMConnectionFile, clusterNetNMConn, 0600); err != nil {
			errors = append(errors, err)
		}
	} else {
		errors = append(errors, err)
	}

	if keepalivedNMConn, err := r.RenderKeepalivedDummyNMConnection(ctx, lbconfig); err == nil {
		if err := r.WriteConfig(ctx, &lbconfig.ObjectMeta, KeepalivedDummyNMConnectionFile, keepalivedNMConn, 0600); err != nil {
			errors = append(errors, err)
		}
	} else {
		errors = append(errors, err)
	}

	if sysctl, err := r.RenderSysctlConf(ctx); err == nil {
		if err := r.WriteConfig(ctx, &lbconfig.ObjectMeta, SysctlConfFile, sysctl, 0644); err != nil {
			errors = append(errors, err)
		}
	} else {
		errors = append(errors, err)
	}

	if zoneext, err := r.RenderFirewallExternalZone(ctx, lbconfig); err == nil {
		if err := r.WriteConfig(ctx, &lbconfig.ObjectMeta, FirewalldExternalZone, zoneext, 0644); err != nil {
			errors = append(errors, err)
		}
	} else {
		errors = append(errors, err)
	}
	if fwdirect, err := r.RenderFirewallDirectRules(ctx, lbconfig); err == nil {
		if err := r.WriteConfig(ctx, &lbconfig.ObjectMeta, FirewalldDirectFile, fwdirect, 0644); err != nil {
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
		Named("loadbalancerconfig").
		For(&lb.LoadBalancerConfig{}).
		Watches(&corev1.Node{}, nodeUpdateHandler{
			client: mgr.GetClient(),
		}).
		Watches(&corev1.Secret{}, credentialsUpdateHandler{
			client: mgr.GetClient(),
		}).
		WithOptions(controller.Options{
			RateLimiter: workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](
				10*time.Second, 5*time.Minute),
		}).
		Complete(r)
}
