// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package operatingsystemconfig

import (
	"bytes"
	"context"
	"slices"
	"time"

	"github.com/go-logr/logr"
	"github.com/spf13/afero"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	predicateutils "github.com/gardener/gardener/pkg/controllerutils/predicate"
	nodeagentv1alpha1 "github.com/gardener/gardener/pkg/nodeagent/apis/config/v1alpha1"
	"github.com/gardener/gardener/pkg/nodeagent/dbus"
	"github.com/gardener/gardener/pkg/nodeagent/registry"
	kubernetesutils "github.com/gardener/gardener/pkg/utils/kubernetes"
)

// ControllerName is the name of this controller.
const ControllerName = "operatingsystemconfig"

// AddToManager adds Reconciler to the given manager.
func (r *Reconciler) AddToManager(ctx context.Context, mgr manager.Manager) error {
	if r.Client == nil {
		r.Client = mgr.GetClient()
	}
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor(ControllerName)
	}
	if r.DBus == nil {
		r.DBus = dbus.New(mgr.GetLogger().WithValues("controller", ControllerName))
	}
	if r.FS.Fs == nil {
		r.FS = afero.Afero{Fs: afero.NewOsFs()}
	}
	if r.Extractor == nil {
		r.Extractor = registry.NewExtractor()
	}

	return builder.
		ControllerManagedBy(mgr).
		Named(ControllerName).
		WatchesRawSource(
			source.Kind(mgr.GetCache(), &corev1.Secret{}),
			r.EnqueueWithJitterDelay(ctx, mgr.GetLogger().WithValues("controller", ControllerName).WithName("reconciliation-delayer")),
			builder.WithPredicates(
				r.SecretPredicate(),
				predicateutils.ForEventTypes(predicateutils.Create, predicateutils.Update),
			),
		).
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(r)
}

// SecretPredicate returns the predicate for Secret events.
func (r *Reconciler) SecretPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(_ event.CreateEvent) bool { return true },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldSecret, ok := e.ObjectOld.(*corev1.Secret)
			if !ok {
				return false
			}

			newSecret, ok := e.ObjectNew.(*corev1.Secret)
			if !ok {
				return false
			}

			return !bytes.Equal(oldSecret.Data[nodeagentv1alpha1.DataKeyOperatingSystemConfig], newSecret.Data[nodeagentv1alpha1.DataKeyOperatingSystemConfig])
		},
		DeleteFunc:  func(_ event.DeleteEvent) bool { return false },
		GenericFunc: func(_ event.GenericEvent) bool { return false },
	}
}

func reconcileRequest(obj client.Object) reconcile.Request {
	return reconcile.Request{NamespacedName: types.NamespacedName{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}}
}

// EnqueueWithJitterDelay returns handler.Funcs which enqueues the object with a random jitter duration for 'update'
// events. 'Create' events are enqueued immediately.
func (r *Reconciler) EnqueueWithJitterDelay(ctx context.Context, log logr.Logger) handler.EventHandler {
	delay := delayer{
		log:             log,
		client:          r.Client,
		minDelaySeconds: 0,
		maxDelaySeconds: int(r.Config.SyncJitterPeriod.Duration.Seconds()),
	}

	return &handler.Funcs{
		CreateFunc: func(_ context.Context, evt event.CreateEvent, q workqueue.RateLimitingInterface) {
			if evt.Object == nil {
				return
			}
			q.Add(reconcileRequest(evt.Object))
		},

		UpdateFunc: func(_ context.Context, evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
			oldSecret, ok := evt.ObjectOld.(*corev1.Secret)
			if !ok {
				return
			}
			newSecret, ok := evt.ObjectNew.(*corev1.Secret)
			if !ok {
				return
			}

			if !bytes.Equal(oldSecret.Data[nodeagentv1alpha1.DataKeyOperatingSystemConfig], newSecret.Data[nodeagentv1alpha1.DataKeyOperatingSystemConfig]) {
				duration := delay.compute(ctx, r.NodeName)
				log.Info("Enqueued secret with operating system config with a jitter period", "duration", duration)
				q.AddAfter(reconcileRequest(evt.ObjectNew), duration)
			}
		},
	}
}

type delayer struct {
	log    logr.Logger
	client client.Client

	minDelaySeconds int
	maxDelaySeconds int

	nodeList *metav1.PartialObjectMetadataList
}

// compute computes a time.Duration that can be used to delay reconciliations by using a simple linear mapping approach
// based on the index of the node this instance of gardener-node-agent is responsible for in the list of all nodes in
// the cluster. This way, the delays of all instances of gardener-node-agent are distributed evenly.
func (d *delayer) compute(ctx context.Context, nodeName string) time.Duration {
	if nodeName == "" {
		return 0
	}

	nodeList := &metav1.PartialObjectMetadataList{}
	nodeList.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("NodeList"))
	if err := d.client.List(ctx, nodeList); err != nil {
		d.log.Error(err, "Failed to list nodes when computing reconciliation delay", "nodeName", nodeName)
		// fall back to previously computed list of nodes
	} else {
		kubernetesutils.ByName().Sort(nodeList)
		d.nodeList = nodeList
	}

	index := slices.IndexFunc(d.nodeList.Items, func(node metav1.PartialObjectMetadata) bool {
		return node.GetName() == nodeName
	})

	rangeSize := float64(d.maxDelaySeconds-d.minDelaySeconds) / float64(len(d.nodeList.Items))
	delaySeconds := float64(d.minDelaySeconds) + float64(index)*rangeSize
	return time.Duration(delaySeconds * float64(time.Second))
}
