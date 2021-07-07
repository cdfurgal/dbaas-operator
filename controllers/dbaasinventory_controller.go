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
	"reflect"

	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/RHEcosystemAppEng/dbaas-operator/api/v1alpha1"
)

// DBaaSInventoryReconciler reconciles a DBaaSInventory object
type DBaaSInventoryReconciler struct {
	*DBaaSReconciler
}

//+kubebuilder:rbac:groups=dbaas.redhat.com,resources=*,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=dbaas.redhat.com,resources=*/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=dbaas.redhat.com,resources=*/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *DBaaSInventoryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx, "DBaaS Inventory", req.NamespacedName)

	var inventory v1alpha1.DBaaSInventory
	if err := r.Get(ctx, req.NamespacedName, &inventory); err != nil {
		if errors.IsNotFound(err) {
			// CR deleted since request queued, child objects getting GC'd, no requeue
			logger.Info("DBaaS Inventory resource not found, has been deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Error fetching DBaaS Inventory for reconcile")
		return ctrl.Result{}, err
	}

	provider, err := r.getDBaaSProvider(inventory.Spec.Provider, req.Namespace, ctx)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Error(err, "Requested DBaaS Provider is not configured in this environment", "DBaaS Provider", provider.Provider)
			return ctrl.Result{}, err
		}
		logger.Error(err, "Error reading configured DBaaS Provider", "DBaaS Provider", inventory.Spec.Provider)
		return ctrl.Result{}, err
	}
	logger.Info("Found DBaaS Provider", "DBaaS Provider", provider.Provider)

	providerInventory, err := r.getProviderObject(&inventory, provider.InventoryKind, ctx)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Provider Inventory resource not found", "DBaaS Provider", inventory.GetName())
			if err = r.createProviderObject(&inventory, provider.InventoryKind, inventory.Spec.DBaaSInventorySpec.DeepCopy(), ctx); err != nil {
				logger.Error(err, "Error creating Provider Inventory resource", "DBaaS Provider", inventory.GetName())
				return ctrl.Result{}, err
			}
			logger.Info("Provider Inventory resource created", "DBaaS Provider", inventory.GetName())
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Error finding the Provider Inventory resource", "DBaaS Provider", inventory.GetName())
		return ctrl.Result{}, err
	}

	providerInventoryStatus, existStatus := providerInventory.UnstructuredContent()["status"]
	if existStatus {
		var status v1alpha1.DBaaSInventoryStatus
		err = decode(providerInventoryStatus, &status)
		if err != nil {
			logger.Error(err, "Error parsing the status of the Provider Inventory resource", "DBaaS Provider", providerInventory.GetName())
			return ctrl.Result{}, err
		}

		inventory.Status = *status.DeepCopy()
		if err = r.Status().Update(ctx, &inventory); err != nil {
			logger.Error(err, "Error updating the DBaaS Inventory status")
			return ctrl.Result{}, err
		}
		logger.Info("DBaaS Inventory status updated")
	} else {
		logger.Info("Provider Inventory resource status not found", "DBaaS Provider", providerInventory.GetName())
	}

	providerInventorySpec, existSpec := providerInventory.UnstructuredContent()["spec"]
	if existSpec {
		var spec v1alpha1.DBaaSInventorySpec
		err = decode(providerInventorySpec, &spec)
		if err != nil {
			logger.Error(err, "Error parsing the spec of the Provider Inventory resource", "DBaaS Provider", providerInventory.GetName())
			return ctrl.Result{}, err
		}

		if !reflect.DeepEqual(spec, inventory.Spec.DBaaSInventorySpec) {
			if err = r.updateProviderObject(providerInventory, inventory.Spec.DBaaSInventorySpec.DeepCopy(), ctx); err != nil {
				logger.Error(err, "Error updating the Provider Inventory spec", "DBaaS Provider", providerInventory.GetName())
				return ctrl.Result{}, err
			}
			logger.Info("Provider Inventory spec updated")
		}
	} else {
		err = fmt.Errorf("failed to get the spec of the Provider Inventory %s", providerInventory.GetName())
		logger.Error(err, "Error getting the spec of the Provider Inventory", "DBaaS Provider", providerInventory.GetName())
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DBaaSInventoryReconciler) SetupWithManager(mgr ctrl.Manager, providerList v1alpha1.DBaaSProviderList) error {
	owned := r.parseDBaaSProviderInventories(providerList)
	builder := ctrl.NewControllerManagedBy(mgr)
	builder = builder.For(&v1alpha1.DBaaSInventory{})
	for _, o := range owned {
		builder = builder.Owns(&o)
	}
	return builder.Complete(r)
}
