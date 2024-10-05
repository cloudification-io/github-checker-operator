/*
Copyright 2024.

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

package controller

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	checkerv1 "github.com/cloudification-io/github-checker-operator/api/v1"
)

// CheckerReconciler reconciles a Checker object
type CheckerReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Clientset *kubernetes.Clientset
}

// +kubebuilder:rbac:groups=checker.cloudification.io,resources=checkers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=checker.cloudification.io,resources=checkers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=checker.cloudification.io,resources=checkers/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch;update;watch;list;get;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Checker object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *CheckerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// TODO(user): your logic here
	log.Log.Info("Trigger reconcile:", "req.NamespacedName", req.NamespacedName)

	// Retrieve the Checker resource (CRD)
	checker := &checkerv1.Checker{}
	if err := r.Get(ctx, req.NamespacedName, checker); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if err := r.ReconcileResources(ctx, &req, checker); err != nil {
		log.Log.Error(err, "Could not create resources", "checker.Name", checker.Name)
	}

	if err := r.UpdateStatus(ctx, &req, checker); err != nil {
		log.Log.Error(err, "Could not create resources", "checker.Name", checker.Name)
	}

	return ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CheckerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&checkerv1.Checker{}).
		Complete(r)
}
