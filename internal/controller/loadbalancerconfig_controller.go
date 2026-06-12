package controller

import (
	"context"
	"crypto/sha256"
	"fmt"
	"maps"
	"net/netip"
	"os"
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
	"sigs.k8s.io/controller-runtime/pkg/predicate"
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
	WatchNamespace  string
	Hostname        string

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
	l := logf.FromContext(ctx)

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

	updated, err := r.ReconcileLBConfig(ctx, &lbconfig, &credentialSecret, nil)
	statuserr := r.Status().Update(ctx, &lbconfig)

	for component := range maps.Keys(updated) {
		l.Info("reloading/restarting component", "component", component)
		component.Reload(ctx)
	}

	return ctrl.Result{}, multierr.Combine(err, statuserr)
}

func (r *LoadBalancerConfigReconciler) ReconcileLBConfig(ctx context.Context, lbconfig *lb.LoadBalancerConfig, credentialSecret *corev1.Secret, localBackends *LocalBackendConfiguration) (map[NodeService]struct{}, error) {
	l := logf.FromContext(ctx)

	updated := map[NodeService]struct{}{}

	l.Info("Reconciling LB config", "cloud", lbconfig.Spec.Cloud, "distribution", lbconfig.Spec.Distribution)

	err := r.InitializeNodeStatus(ctx, lbconfig)
	if err != nil {
		return updated, fmt.Errorf("failed to initialize node status: %w", err)
	}

	errors := []error{}

	if haproxyApi, err := r.RenderHAProxyAPIConfig(ctx, lbconfig, localBackends); err == nil {
		changed, err := r.CompareAndWrite(ctx, lbconfig, HAProxyAPIConfigFile, haproxyApi, 0644)
		if err != nil {
			errors = append(errors, err)
		} else if changed {
			updated[HAProxy] = struct{}{}
		}
	} else {
		errors = append(errors, err)
	}

	if haproxyIngress, err := r.RenderHAProxyIngressConfig(ctx, lbconfig, localBackends); err == nil {
		changed, err := r.CompareAndWrite(ctx, lbconfig, HAProxyIngressConfigFile, haproxyIngress, 0644)
		if err != nil {
			errors = append(errors, err)
		} else if changed {
			updated[HAProxy] = struct{}{}
		}
	} else {
		errors = append(errors, err)
	}

	if keepalived, err := r.RenderKeepalivedConfig(ctx, lbconfig); err == nil {
		changed, err := r.CompareAndWrite(ctx, lbconfig, KeepalivedConfigFile, keepalived, 0644)
		if err != nil {
			errors = append(errors, err)
		} else if changed {
			updated[Keepalived] = struct{}{}
		}
	} else {
		errors = append(errors, err)
	}

	if floaty, err := r.RenderFloatyConfig(ctx, lbconfig, credentialSecret); err == nil {
		changed, err := r.CompareAndWrite(ctx, lbconfig, FloatyConfigFile, floaty, 0644)
		if err != nil {
			errors = append(errors, err)
		} else if changed {
			updated[Keepalived] = struct{}{}
		}
	} else {
		errors = append(errors, err)
	}

	if conntrackd, err := r.RenderConntrackdConfig(ctx, lbconfig); err == nil {
		changed, err := r.CompareAndWrite(ctx, lbconfig, ConntrackdConfigFile, conntrackd, 0644)
		if err != nil {
			errors = append(errors, err)
		} else if changed {
			updated[Conntrackd] = struct{}{}
		}
	} else {
		errors = append(errors, err)
	}

	if publicNMConn, err := r.RenderPublicNMConnection(ctx, lbconfig); err == nil {
		changed, err := r.CompareAndWrite(ctx, lbconfig, PublicNMConnectionFile, publicNMConn, 0600)
		if err != nil {
			errors = append(errors, err)
		} else if changed {
			updated[NetworkManager] = struct{}{}
		}
	} else {
		errors = append(errors, err)
	}

	if clusterNetNMConn, err := r.RenderClusterNetNMConnection(ctx); err == nil {
		changed, err := r.CompareAndWrite(ctx, lbconfig, ClusterNetworkNMConnectionFile, clusterNetNMConn, 0600)
		if err != nil {
			errors = append(errors, err)
		} else if changed {
			updated[NetworkManager] = struct{}{}
		}
	} else {
		errors = append(errors, err)
	}

	if keepalivedNMConn, err := r.RenderKeepalivedDummyNMConnection(ctx, lbconfig); err == nil {
		changed, err := r.CompareAndWrite(ctx, lbconfig, KeepalivedDummyNMConnectionFile, keepalivedNMConn, 0600)
		if err != nil {
			errors = append(errors, err)
		} else if changed {
			updated[NetworkManager] = struct{}{}
		}
	} else {
		errors = append(errors, err)
	}

	if sysctl, err := r.RenderSysctlConf(ctx); err == nil {
		changed, err := r.CompareAndWrite(ctx, lbconfig, SysctlConfFile, sysctl, 0644)
		if err != nil {
			errors = append(errors, err)
		} else if changed {
			updated[Sysctl] = struct{}{}
		}
	} else {
		errors = append(errors, err)
	}

	if zoneext, err := r.RenderFirewallExternalZone(ctx, lbconfig); err == nil {
		changed, err := r.CompareAndWrite(ctx, lbconfig, FirewalldExternalZone, zoneext, 0644)
		if err != nil {
			errors = append(errors, err)
		} else if changed {
			updated[Firewalld] = struct{}{}
		}
	} else {
		errors = append(errors, err)
	}
	if fwdirect, err := r.RenderFirewallDirectRules(ctx, lbconfig); err == nil {
		changed, err := r.CompareAndWrite(ctx, lbconfig, FirewalldDirectFile, fwdirect, 0644)
		if err != nil {
			errors = append(errors, err)
		} else if changed {
			updated[Firewalld] = struct{}{}
		}
	} else {
		errors = append(errors, err)
	}
	if len(errors) == 0 {
		lbconfig.Status.Nodes[r.Hostname].Status = "Ready"
	}

	return updated, multierr.Combine(errors...)
}

func (r *LoadBalancerConfigReconciler) InitializeNodeStatus(ctx context.Context, lbconfig *lb.LoadBalancerConfig) error {
	internalIPs, err := r.internalIPs()
	if err != nil {
		return fmt.Errorf("failed to compute internal IPs: %w", err)
	}
	publicIP, err := primaryIPAddressForInterface(r.PublicInterface, nil)
	if err != nil {
		return fmt.Errorf("failed to determine primary IP for public interface: %w", err)
	}
	clusterIface, err := r.ClusterNetworkInterface(ctx)
	if err != nil {
		return fmt.Errorf("failed to find cluster network interface: %w", err)
	}
	privateIP, err := primaryIPAddressForInterface(clusterIface, &r.ClusterNetwork)
	if err != nil {
		return fmt.Errorf("failed to determine primary IP for cluster network interface: %w", err)
	}

	if lbconfig.Status.Nodes == nil {
		lbconfig.Status.Nodes = map[string]*lb.LoadBalancerNodeStatus{}
	}
	if lbconfig.Status.Nodes[r.Hostname] == nil {
		lbconfig.Status.Nodes[r.Hostname] = &lb.LoadBalancerNodeStatus{
			Status:    lb.NodeStatusNotReady,
			PublicIP:  publicIP,
			PrivateIP: privateIP,
			VrrpIP:    internalIPs.myInternalIP(r.KeepalivedConfig.IsPrimary),
		}
	}
	return nil

}

func (r *LoadBalancerConfigReconciler) CompareAndWrite(ctx context.Context, lbconfig *lb.LoadBalancerConfig, configfile, configdata string, mode os.FileMode) (bool, error) {
	l := logf.FromContext(ctx)

	nodeStatus := lbconfig.Status.Nodes[r.Hostname]
	if nodeStatus.ConfigHashes == nil {
		nodeStatus.ConfigHashes = map[string]string{}
	}

	dataDigest := fmt.Sprintf("%x", sha256.Sum256([]byte(configdata)))

	l.Info("live vs new sha256 digest", "configfile", configfile, "live sha256", nodeStatus.ConfigHashes[configfile], "new sha256", dataDigest)
	if nodeStatus.ConfigHashes[configfile] == dataDigest {
		return false, nil
	}
	nodeStatus.ConfigHashes[configfile] = dataDigest
	lbconfig.Status.Nodes[r.Hostname] = nodeStatus

	return true, r.WriteConfig(ctx, &lbconfig.ObjectMeta, configfile, configdata, mode)
}

func (r *LoadBalancerConfigReconciler) Filter(obj client.Object) bool {
	switch obj.(type) {
	case *lb.LoadBalancerConfig:
		return r.WatchNamespace != "" && r.WatchNamespace == obj.GetNamespace()
	case *corev1.Node:
		return true
	case *corev1.Secret:
		return r.WatchNamespace != "" && r.WatchNamespace == obj.GetNamespace()
	}
	return false
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
		WithEventFilter(predicate.NewPredicateFuncs(r.Filter)).
		WithOptions(controller.Options{
			RateLimiter: workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](
				10*time.Second, 5*time.Minute),
		}).
		Complete(r)
}
