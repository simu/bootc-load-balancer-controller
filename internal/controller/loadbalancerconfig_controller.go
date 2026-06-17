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

	lb "github.com/simu/bootc-load-balancer-controller/api/v1alpha1"
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
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch

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

	errors := []error{}

	updated, err := r.ReconcileLBConfig(ctx, &lbconfig, &credentialSecret, nil)
	if err != nil {
		errors = append(errors, err)
	}

	for component := range maps.Keys(updated) {
		l.Info("reloading/restarting component", "component", component)
		if r.ConfigRoot != "/" {
			cmd, err := component.ReloadCommand()
			if err != nil {
				cmd = fmt.Sprintf("failed to render reload command: %s", err)
			}
			l.Info("not trying to restart service because config-root != /", "config-root", r.ConfigRoot, "command", cmd)
		} else {
			err := component.Reload(ctx)
			if err != nil {
				errors = append(errors, err)
			}
		}
	}

	if len(errors) == 0 {
		lbconfig.Status.Nodes[r.Hostname].Status = lb.NodeStatusReady
	} else {
		lbconfig.Status.Nodes[r.Hostname].Status = lb.NodeStatusFailed
	}
	statuserr := r.Status().Update(ctx, &lbconfig)

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

	if changed, err := r.RenderConfig(ctx, r.RenderKeepalivedConfig, lbconfig, KeepalivedConfigFile, 0644); err != nil {
		errors = append(errors, err)
	} else if changed {
		updated[Keepalived] = struct{}{}
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

	if changed, err := r.RenderConfig(ctx, r.RenderConntrackdConfig, lbconfig, ConntrackdConfigFile, 0644); err != nil {
		errors = append(errors, err)
	} else if changed {
		updated[Conntrackd] = struct{}{}
	}

	if changed, err := r.RenderConfig(ctx, r.RenderPublicNMConnection, lbconfig, PublicNMConnectionFile, 0600); err != nil {
		errors = append(errors, err)
	} else if changed {
		updated[NetworkManager] = struct{}{}
	}

	if changed, err := r.RenderConfig(ctx, r.RenderClusterNetNMConnection, lbconfig, ClusterNetworkNMConnectionFile, 0600); err != nil {
		errors = append(errors, err)
	} else if changed {
		updated[NetworkManager] = struct{}{}
	}

	if changed, err := r.RenderConfig(ctx, r.RenderKeepalivedDummyNMConnection, lbconfig, KeepalivedDummyNMConnectionFile, 0600); err != nil {
		errors = append(errors, err)
	} else if changed {
		updated[NetworkManager] = struct{}{}
	}

	if changed, err := r.RenderConfig(ctx, r.RenderSysctlConf, lbconfig, SysctlConfFile, 0644); err != nil {
		errors = append(errors, err)
	} else if changed {
		updated[Sysctl] = struct{}{}
	}

	if changed, err := r.RenderConfig(ctx, r.RenderFirewallExternalZone, lbconfig, FirewalldExternalZone, 0644); err != nil {
		errors = append(errors, err)
	} else if changed {
		updated[Firewalld] = struct{}{}
	}
	if changed, err := r.RenderConfig(ctx, r.RenderFirewallDirectRules, lbconfig, FirewalldDirectFile, 0644); err != nil {
		errors = append(errors, err)
	} else if changed {
		updated[Firewalld] = struct{}{}
	}

	return updated, multierr.Combine(errors...)
}

func (r *LoadBalancerConfigReconciler) RenderConfig(ctx context.Context, renderFunc func(context.Context, *lb.LoadBalancerConfig) (string, error), lbconfig *lb.LoadBalancerConfig, configFile string, mode os.FileMode) (bool, error) {
	config, err := renderFunc(ctx, lbconfig)
	if err != nil {
		return false, err
	}
	return r.CompareAndWrite(ctx, lbconfig, configFile, config, mode)
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
			Status:    lb.NodeStatusConfiguring,
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

	isDiskModified := r.IsFileModified(ctx, configfile)

	l.Info("file modification comparison inputs", "configfile", configfile, "cr status sha256", nodeStatus.ConfigHashes[configfile], "data sha256", dataDigest, "on-disk modified", isDiskModified)
	if !isDiskModified && nodeStatus.ConfigHashes[configfile] == dataDigest {
		return false, nil
	}
	nodeStatus.ConfigHashes[configfile] = dataDigest
	lbconfig.Status.Nodes[r.Hostname] = nodeStatus

	return true, r.WriteConfig(ctx, &lbconfig.ObjectMeta, configfile, configdata, dataDigest, mode)
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
