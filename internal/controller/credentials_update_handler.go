package controller

import (
	"context"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	lb "github.com/simu/bootc-load-balancer-controller/api/v1alpha1"
)

type credentialsUpdateHandler struct {
	client client.Client
}

func (h credentialsUpdateHandler) Create(ctx context.Context, ev event.TypedCreateEvent[client.Object], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// No need to requeue all LBConfig on the initial list. All LBConfig
	// will be queued anyways on startup.
	if ev.IsInInitialList {
		return
	}

	h.queueAllLBConfigs(ctx, q)
}

// Update requeues all LBConfig objects
func (h credentialsUpdateHandler) Update(ctx context.Context, ev event.TypedUpdateEvent[client.Object], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// TODO(sg): we should filter this somehow -> for realz, we probably
	// want to run a dynamic watch on only secrets referenced by LBConfigs

	h.queueAllLBConfigs(ctx, q)
}

func (h credentialsUpdateHandler) Delete(ctx context.Context, ev event.TypedDeleteEvent[client.Object], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.queueAllLBConfigs(ctx, q)
}

func (h credentialsUpdateHandler) Generic(ctx context.Context, ev event.TypedGenericEvent[client.Object], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.queueAllLBConfigs(ctx, q)
}

func (h credentialsUpdateHandler) queueAllLBConfigs(ctx context.Context, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	var lbconfigs lb.LoadBalancerConfigList
	if err := h.client.List(ctx, &lbconfigs); err != nil {
		log.FromContext(ctx).Error(err, "Failed to list LBConfigs")
		return
	}

	for _, lbconfig := range lbconfigs.Items {
		q.Add(reconcile.Request{
			NamespacedName: client.ObjectKey{
				Namespace: lbconfig.Namespace,
				Name:      lbconfig.Name,
			},
		})
	}
}
