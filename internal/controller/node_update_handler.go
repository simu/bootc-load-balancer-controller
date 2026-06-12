package controller

import (
	"context"
	"maps"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	lb "github.com/projectsyn/bootc-load-balancer-controller/api/v1alpha1"
)

type nodeUpdateHandler struct {
	client client.Client
}

func (h nodeUpdateHandler) Create(ctx context.Context, ev event.TypedCreateEvent[client.Object], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// No need to requeue all LBConfig on the initial list. All LBConfig
	// will be queued anyways on startup.
	if ev.IsInInitialList {
		return
	}

	h.queueAllLBConfigs(ctx, q)
}

// Update only requeues all LBConfig objects if the labels of a node have changed.
func (h nodeUpdateHandler) Update(ctx context.Context, ev event.TypedUpdateEvent[client.Object], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	o := ev.ObjectOld.(*corev1.Node)
	n := ev.ObjectNew.(*corev1.Node)
	if maps.Equal(o.GetLabels(), n.GetLabels()) {
		return
	}

	h.queueAllLBConfigs(ctx, q)
}

func (h nodeUpdateHandler) Delete(ctx context.Context, ev event.TypedDeleteEvent[client.Object], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.queueAllLBConfigs(ctx, q)
}

func (h nodeUpdateHandler) Generic(ctx context.Context, ev event.TypedGenericEvent[client.Object], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	h.queueAllLBConfigs(ctx, q)
}

func (h nodeUpdateHandler) queueAllLBConfigs(ctx context.Context, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
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
