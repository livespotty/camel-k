/*
Licensed to the Apache Software Foundation (ASF) under one or more
contributor license agreements.  See the NOTICE file distributed with
this work for additional information regarding copyright ownership.
The ASF licenses this file to You under the Apache License, Version 2.0
(the "License"); you may not use this file except in compliance with
the License.  You may obtain a copy of the License at

   http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package integration

import (
	"context"
	"fmt"

	"reflect"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"knative.dev/serving/pkg/apis/serving"
	servingv1 "knative.dev/serving/pkg/apis/serving/v1"

	v1 "github.com/apache/camel-k/v2/pkg/apis/camel/v1"
	"github.com/apache/camel-k/v2/pkg/client"
	camelevent "github.com/apache/camel-k/v2/pkg/event"
	"github.com/apache/camel-k/v2/pkg/platform"
	"github.com/apache/camel-k/v2/pkg/trait"
	"github.com/apache/camel-k/v2/pkg/util/digest"
	"github.com/apache/camel-k/v2/pkg/util/kubernetes"
	"github.com/apache/camel-k/v2/pkg/util/log"
	"github.com/apache/camel-k/v2/pkg/util/monitoring"
	utilResource "github.com/apache/camel-k/v2/pkg/util/resource"
)

const retryMonitoring = 5

func Add(ctx context.Context, mgr manager.Manager, c client.Client) error {
	err := mgr.GetFieldIndexer().IndexField(ctx, &corev1.Pod{}, "status.phase",
		func(obj ctrl.Object) []string {
			pod, _ := obj.(*corev1.Pod)
			return []string{string(pod.Status.Phase)}
		})

	if err != nil {
		return fmt.Errorf("unable to set up field indexer for status.phase: %w", err)
	}

	return add(ctx, mgr, c, newReconciler(mgr, c))
}

func newReconciler(mgr manager.Manager, c client.Client) reconcile.Reconciler {
	return monitoring.NewInstrumentedReconciler(
		&reconcileIntegration{
			client:   c,
			scheme:   mgr.GetScheme(),
			recorder: mgr.GetEventRecorderFor("camel-k-integration-controller"),
		},
		schema.GroupVersionKind{
			Group:   v1.SchemeGroupVersion.Group,
			Version: v1.SchemeGroupVersion.Version,
			Kind:    v1.IntegrationKind,
		},
	)
}

func integrationUpdateFunc(c client.Client, old *v1.Integration, it *v1.Integration) bool {
	// Observe the time to first readiness metric
	previous := old.Status.GetCondition(v1.IntegrationConditionReady)
	next := it.Status.GetCondition(v1.IntegrationConditionReady)
	if isIntegrationUpdated(it, previous, next) {
		duration := next.FirstTruthyTime.Time.Sub(it.Status.InitializationTimestamp.Time)
		Log.WithValues("request-namespace", it.Namespace, "request-name", it.Name, "ready-after", duration.Seconds()).
			ForIntegration(it).Infof("First readiness after %s", duration)
		timeToFirstReadiness.Observe(duration.Seconds())
	}

	updateIntegrationPhase(it.Name, string(it.Status.Phase))
	// If traits have changed, the reconciliation loop must kick in as
	// traits may have impact
	sameTraits, err := trait.IntegrationsHaveSameTraits(c, old, it)
	if err != nil {
		Log.ForIntegration(it).Error(
			err,
			"unable to determine if old and new resource have the same traits")
	}
	if !sameTraits {
		return true
	}

	// Ignore updates to the integration status in which case metadata.Generation does not change,
	// or except when the integration phase changes as it's used to transition from one phase
	// to another.
	return old.Generation != it.Generation ||
		old.Status.Phase != it.Status.Phase
}

func isIntegrationUpdated(it *v1.Integration, previous, next *v1.IntegrationCondition) bool {
	if previous == nil || previous.Status != corev1.ConditionTrue && (previous.FirstTruthyTime == nil || previous.FirstTruthyTime.IsZero()) {
		if next != nil && next.Status == corev1.ConditionTrue && next.FirstTruthyTime != nil && !next.FirstTruthyTime.IsZero() {
			return it.Status.InitializationTimestamp != nil
		}
	}

	return false
}

func integrationKitEnqueueRequestsFromMapFunc(ctx context.Context, c client.Client, kit *v1.IntegrationKit) []reconcile.Request {
	requests := make([]reconcile.Request, 0)
	if kit.Status.Phase != v1.IntegrationKitPhaseReady && kit.Status.Phase != v1.IntegrationKitPhaseError {
		return requests
	}

	list := &v1.IntegrationList{}
	// Do global search in case of global operator (it may be using a global platform)
	var opts []ctrl.ListOption
	if !platform.IsCurrentOperatorGlobal() {
		opts = append(opts, ctrl.InNamespace(kit.Namespace))
	}
	if err := c.List(ctx, list, opts...); err != nil {
		log.Error(err, "Failed to retrieve integration list")
		return requests
	}

	for i := range list.Items {
		integration := &list.Items[i]
		if integration.Status.Phase != v1.IntegrationPhaseBuildingKit &&
			integration.Status.Phase != v1.IntegrationPhaseRunning {
			continue
		}

		Log.Debug("Integration Controller: Assessing integration", "integration", integration.Name, "namespace", integration.Namespace)

		match, err := sameOrMatch(ctx, c, kit, integration)
		if err != nil {
			Log.ForIntegration(integration).Errorf(err, "Error matching integration %q with kit %q", integration.Name, kit.Name)
			continue
		}
		if !match {
			continue
		}

		log.Infof("Kit %s ready, notify integration: %s", kit.Name, integration.Name)
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: integration.Namespace,
				Name:      integration.Name,
			},
		})
	}

	return requests
}

func enqueueRequestsFromConfigFunc(ctx context.Context, c client.Client, res ctrl.Object) []reconcile.Request {
	requests := make([]reconcile.Request, 0)

	var storageType utilResource.StorageType

	switch res.(type) {
	case *corev1.ConfigMap:
		storageType = utilResource.StorageTypeConfigmap
	case *corev1.Secret:
		storageType = utilResource.StorageTypeSecret
	default:
		return requests
	}

	// Do global search in case of global operator (it may be using a global platform)
	list := &v1.IntegrationList{}

	opts := make([]ctrl.ListOption, 0)
	if !platform.IsCurrentOperatorGlobal() {
		opts = append(opts, ctrl.InNamespace(res.GetNamespace()))
	}

	if err := c.List(ctx, list, opts...); err != nil {
		log.Error(err, "Failed to list integrations")
		return requests
	}

	for _, integration := range list.Items {
		found := false
		if integration.Spec.Traits.Mount == nil || !ptr.Deref(integration.Spec.Traits.Mount.HotReload, false) {
			continue
		}
		for _, c := range integration.Spec.Traits.Mount.Configs {
			if conf, parseErr := utilResource.ParseConfig(c); parseErr == nil {
				if conf.StorageType() == storageType && conf.Name() == res.GetName() {
					found = true
					break
				}
			}
		}
		for _, r := range integration.Spec.Traits.Mount.Resources {
			if conf, parseErr := utilResource.ParseConfig(r); parseErr == nil {
				if conf.StorageType() == storageType && conf.Name() == res.GetName() {
					found = true
					break
				}
			}
		}

		if found {
			log.Infof("%s %s updated, wake-up integration: %s", res.GetObjectKind(), res.GetName(), integration.Name)
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: integration.Namespace,
					Name:      integration.Name,
				},
			})
		}
	}

	return requests
}

func integrationPlatformEnqueueRequestsFromMapFunc(ctx context.Context, c client.Client, p *v1.IntegrationPlatform) []reconcile.Request {
	var requests []reconcile.Request

	if p.Status.Phase == v1.IntegrationPlatformPhaseReady {
		list := &v1.IntegrationList{}

		// Do global search in case of global operator (it may be using a global platform)
		var opts []ctrl.ListOption
		if !platform.IsCurrentOperatorGlobal() {
			opts = append(opts, ctrl.InNamespace(p.Namespace))
		}

		if err := c.List(ctx, list, opts...); err != nil {
			log.Error(err, "Failed to list integrations")
			return requests
		}

		for _, integration := range list.Items {
			if integration.Status.Phase == v1.IntegrationPhaseWaitingForPlatform {
				log.Infof("Platform %s ready, wake-up integration: %s", p.Name, integration.Name)
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: integration.Namespace,
						Name:      integration.Name,
					},
				})
			}
		}
	}

	return requests
}

func add(ctx context.Context, mgr manager.Manager, c client.Client, r reconcile.Reconciler) error {
	b := builder.ControllerManagedBy(mgr).
		Named("integration-controller").
		// Watch for changes to primary resource Integration
		For(&v1.Integration{}, builder.WithPredicates(
			platform.FilteringFuncs[ctrl.Object]{
				UpdateFunc: func(e event.UpdateEvent) bool {
					old, ok := e.ObjectOld.(*v1.Integration)
					if !ok {
						return false
					}
					it, ok := e.ObjectNew.(*v1.Integration)
					if !ok {
						return false
					}

					return integrationUpdateFunc(c, old, it)
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					// Evaluates to false if the object has been confirmed deleted
					return !e.DeleteStateUnknown
				},
			}))

	// Watch for all the resources
	watchIntegrationResources(c, b)
	// Watch for the CronJob conditionally
	if ok, err := kubernetes.IsAPIResourceInstalled(c, batchv1.SchemeGroupVersion.String(), reflect.TypeOf(batchv1.CronJob{}).Name()); ok && err == nil {
		watchCronJobResources(b)
	}
	// Watch for the Knative Services conditionally
	if ok, err := kubernetes.IsAPIResourceInstalled(c, servingv1.SchemeGroupVersion.String(), reflect.TypeOf(servingv1.Service{}).Name()); err != nil {
		return err
	} else if ok {
		if err = watchKnativeResources(ctx, c, b); err != nil {
			return err
		}
	}

	return b.Complete(r)
}

func watchIntegrationResources(c client.Client, b *builder.Builder) {
	// Watch for IntegrationKit phase transitioning to ready or error, and
	// enqueue requests for any integration that matches the kit, in building
	// or running phase.
	b.Watches(&v1.IntegrationKit{},
		handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, a ctrl.Object) []reconcile.Request {
			kit, ok := a.(*v1.IntegrationKit)
			if !ok {
				log.Error(fmt.Errorf("type assertion failed: %v", a), "Failed to retrieve IntegrationKit")
				return []reconcile.Request{}
			}
			return integrationKitEnqueueRequestsFromMapFunc(ctx, c, kit)
		})).
		// Watch for IntegrationPlatform phase transitioning to ready and enqueue
		// requests for any integrations that are in phase waiting for platform
		Watches(&v1.IntegrationPlatform{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, a ctrl.Object) []reconcile.Request {
				p, ok := a.(*v1.IntegrationPlatform)
				if !ok {
					log.Error(fmt.Errorf("type assertion failed: %v", a), "Failed to retrieve IntegrationPlatform")
					return []reconcile.Request{}
				}
				return integrationPlatformEnqueueRequestsFromMapFunc(ctx, c, p)
			})).
		// Watch for Configmaps or Secret used in the Integrations for updates
		Watches(&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, a ctrl.Object) []reconcile.Request {
				cm, ok := a.(*corev1.ConfigMap)
				if !ok {
					log.Error(fmt.Errorf("type assertion failed: %v", a), "Failed to retrieve to retrieve Configmap")
					return []reconcile.Request{}
				}
				return enqueueRequestsFromConfigFunc(ctx, c, cm)
			}),
			builder.WithPredicates(predicate.NewPredicateFuncs(func(object ctrl.Object) bool {
				return object.GetLabels()["camel.apache.org/integration"] != ""
			})),
		).
		Watches(&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, a ctrl.Object) []reconcile.Request {
				secret, ok := a.(*corev1.Secret)
				if !ok {
					log.Error(fmt.Errorf("type assertion failed: %v", a), "Failed to retrieve to retrieve Secret")
					return []reconcile.Request{}
				}
				return enqueueRequestsFromConfigFunc(ctx, c, secret)
			}),
			builder.WithPredicates(predicate.NewPredicateFuncs(func(object ctrl.Object) bool {
				return object.GetLabels()["camel.apache.org/integration"] != ""
			})),
		).
		// Watch for the Integration Pods belonging to managed Integrations
		Watches(&corev1.Pod{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, a ctrl.Object) []reconcile.Request {
				pod, ok := a.(*corev1.Pod)
				if !ok {
					log.Error(fmt.Errorf("type assertion failed: %v", a), "Failed to retrieve to retrieve Pod")
					return []reconcile.Request{}
				}
				if pod.Labels[v1.IntegrationLabel] == "" {
					return []reconcile.Request{}
				}
				return []reconcile.Request{
					{
						NamespacedName: types.NamespacedName{
							Namespace: pod.GetNamespace(),
							Name:      pod.Labels[v1.IntegrationLabel],
						},
					},
				}
			})).
		// Watch for the owned Deployments
		Owns(&appsv1.Deployment{}, builder.WithPredicates(StatusChangedPredicate{})).
		// Watch for the owned Builds
		Owns(&v1.Build{}, builder.WithPredicates(StatusChangedPredicate{}))
}

func watchCronJobResources(b *builder.Builder) {
	// Watch for the owned CronJobs
	b.Owns(&batchv1.CronJob{}, builder.WithPredicates(StatusChangedPredicate{}))
}

func watchKnativeResources(ctx context.Context, c client.Client, b *builder.Builder) error {
	// Watch for the owned Knative Services conditionally
	ok, err := kubernetes.IsAPIResourceInstalled(c, servingv1.SchemeGroupVersion.String(), reflect.TypeOf(servingv1.Service{}).Name())
	if err != nil {
		return err
	}
	if !ok {
		log.Info(`KnativeService resources are not installed in the cluster. You can't use Knative features. If you install Knative Serving resources after the
			Camel K operator, make sure to apply the required RBAC privileges and restart the Camel K Operator Pod to be able to watch for
			Camel K managed Knative Services.`)

		return nil
	}

	// Check for permission to watch the Knative Service resource
	checkCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if ok, err = kubernetes.CheckPermission(checkCtx, c, serving.GroupName, "services", platform.GetOperatorWatchNamespace(), "", "watch"); err != nil {
		return err
	} else if ok {
		log.Info("KnativeService resources installed in the cluster. RBAC privileges assigned correctly, you can use Knative features.")
		b.Owns(&servingv1.Service{}, builder.WithPredicates(StatusChangedPredicate{}))
	} else {
		log.Info(` KnativeService resources installed in the cluster. However Camel K operator has not the required RBAC privileges. You can't use Knative features.
				Make sure to apply the required RBAC privileges and restart the Camel K Operator Pod to be able to watch for Camel K managed Knative Services.`)
	}

	return nil
}

var _ reconcile.Reconciler = &reconcileIntegration{}

// reconcileIntegration reconciles an Integration object.
type reconcileIntegration struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the API server
	client   client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
}

// Reconcile reads that state of the cluster for an Integration object and makes changes based on the state read
// and what is in the Integration.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *reconcileIntegration) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	rlog := Log.WithValues("request-namespace", request.Namespace, "request-name", request.Name)
	rlog.Debug("Reconciling Integration")

	// Make sure the operator is allowed to act on namespace
	if ok, err := platform.IsOperatorAllowedOnNamespace(ctx, r.client, request.Namespace); err != nil {
		return reconcile.Result{}, err
	} else if !ok {
		rlog.Info("Ignoring request because namespace is locked")
		return reconcile.Result{}, nil
	}

	// Fetch the Integration instance
	var instance v1.Integration

	if err := r.client.Get(ctx, request.NamespacedName, &instance); err != nil {
		if k8serrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Only process resources assigned to the operator
	if !platform.IsOperatorHandlerConsideringLock(ctx, r.client, request.Namespace, &instance) {
		rlog.Info("Ignoring request because resource is not assigned to current operator")
		return reconcile.Result{}, nil
	}

	target := instance.DeepCopy()
	targetLog := rlog.ForIntegration(target)

	actions := []Action{
		NewPlatformSetupAction(),
		NewInitializeAction(),
		NewBuildAction(),
		newBuildKitAction(),
	}

	if instance.IsSynthetic() {
		actions = append(actions, NewMonitorSyntheticAction())
	} else {
		actions = append(actions, NewMonitorAction(), NewMonitorUnknownAction())
	}

	for _, a := range actions {
		a.InjectClient(r.client)
		a.InjectLogger(targetLog)

		if !a.CanHandle(target) {
			continue
		}

		targetLog.Debugf("Invoking action %s", a.Name())

		newTarget, err := a.Handle(ctx, target)
		if err != nil {
			camelevent.NotifyIntegrationError(ctx, r.client, r.recorder, &instance, newTarget, err)
			// Update the integration (mostly just to update its phase) if the new instance is returned
			if newTarget != nil {
				_ = r.update(ctx, &instance, newTarget, &targetLog)
			}
			return reconcile.Result{}, err
		}

		if newTarget != nil {
			if err := r.update(ctx, &instance, newTarget, &targetLog); err != nil {
				camelevent.NotifyIntegrationError(ctx, r.client, r.recorder, &instance, newTarget, err)
				return reconcile.Result{}, err
			}

			if newTarget.Status.Phase == v1.IntegrationPhaseUnknown {
				// Wait for some time before trying to monitor again
				return reconcile.Result{RequeueAfter: retryMonitoring * time.Second}, nil
			}
		}

		// handle one action at time so the resource
		// is always at its latest state
		camelevent.NotifyIntegrationUpdated(ctx, r.client, r.recorder, &instance, newTarget)

		break
	}

	return reconcile.Result{}, nil
}

func (r *reconcileIntegration) update(ctx context.Context, base *v1.Integration, target *v1.Integration, log *log.Logger) error {
	secrets, configmaps := getIntegrationSecretAndConfigmapResourceVersions(ctx, r.client, target)
	d, err := digest.ComputeForIntegration(target, configmaps, secrets)
	if err != nil {
		return err
	}

	target.Status.Digest = d
	target.Status.ObservedGeneration = base.Generation

	if err := r.client.Status().Patch(ctx, target, ctrl.MergeFrom(base)); err != nil {
		return err
	}

	if target.Status.Phase != base.Status.Phase {
		log.Info(
			"State transition",
			"phase-from", base.Status.Phase,
			"phase-to", target.Status.Phase,
		)

		if target.Status.Phase == v1.IntegrationPhaseError {
			if cond := target.Status.GetCondition(v1.IntegrationConditionReady); cond != nil && cond.Status == corev1.ConditionFalse {
				log.Info(
					"Integration error",
					"reason", cond.GetReason(),
					"error-message", cond.GetMessage())
			}
		}
	}

	return nil
}
