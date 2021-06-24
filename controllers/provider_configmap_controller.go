/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/google/uuid"
)

const (
	deploymentProviderSyncKey = "provider-sync-uuid"

	deploymentLabelKey   = "dbaas-operator-resource"
	deploymentLabelValue = "dbaas-operator-deployment"
)

// ProviderConfigMapReconciler reconciles Provider's ConfigMap object
type ProviderConfigMapReconciler struct {
	client.Client
	*runtime.Scheme
}

//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *ProviderConfigMapReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx, "Provider ConfigMap", req.NamespacedName)

	var deploymentList appsv1.DeploymentList
	opts := []client.ListOption{
		client.InNamespace(req.Namespace),
		client.MatchingLabels{
			deploymentLabelKey: deploymentLabelValue,
		},
	}

	if err := r.List(ctx, &deploymentList, opts...); err != nil {
		logger.Error(err, "Error fetching the operator's Deployment")
		return ctrl.Result{}, err
	} else if len(deploymentList.Items) != 1 {
		err = fmt.Errorf("found %d Deployments that match operator's Deployment", len(deploymentList.Items))
		return ctrl.Result{}, err
	}

	deployment := deploymentList.Items[0]
	if deployment.Spec.Template.Annotations == nil {
		deployment.Spec.Template.Annotations = map[string]string{deploymentProviderSyncKey: uuid.New().String()}
	} else {
		deployment.Spec.Template.Annotations[deploymentProviderSyncKey] = uuid.New().String()
	}
	if err := r.Update(ctx, &deployment); err != nil {
		logger.Error(err, "Error updating the operator's Deployment to reload providers")
	}
	logger.Info("Operator's Deployment updated, operator will restart")

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProviderConfigMapReconciler) SetupWithManager(mgr ctrl.Manager, cmList v1.ConfigMapList) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.ConfigMap{}).
		WithEventFilter(filterEventPredicate(cmList)).
		Complete(r)
}

func filterEventPredicate(cmList v1.ConfigMapList) predicate.Predicate {
	cmNames := make([]string, len(cmList.Items))
	for i, cm := range cmList.Items {
		cmNames[i] = cm.Name
	}

	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			for _, n := range cmNames {
				if n == e.Object.GetName() {
					return false
				}
			}
			return true
		},
	}
}
