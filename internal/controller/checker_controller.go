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

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	checkerv1 "github.com/cloudification-io/github-checker-operator/api/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CheckerReconciler reconciles a Checker object
type CheckerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=checker.cloudification.io,resources=checkers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=checker.cloudification.io,resources=checkers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=checker.cloudification.io,resources=checkers/finalizers,verbs=update

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

	// Retrieve the Checker resource (CRD)
	checker := &checkerv1.Checker{}
	if err := r.Get(ctx, req.NamespacedName, checker); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	cronJob := &batchv1.CronJob{}
	cronJobName := types.NamespacedName{Name: checker.ObjectMeta.Name, Namespace: req.Namespace}

	if err := r.Get(ctx, cronJobName, cronJob); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}

		log.Log.Info("Creating CronJob for Checker", "Checker.Name", checker.Name)

		newCronJob := &batchv1.CronJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      checker.ObjectMeta.Name,
				Namespace: req.Namespace,
				Labels: map[string]string{
					"app": "curl-checker",
				},
			},
			Spec: batchv1.CronJobSpec{
				Schedule:                   "* * * * *",
				ConcurrencyPolicy:          batchv1.ForbidConcurrent,
				SuccessfulJobsHistoryLimit: int32Ptr(1),
				FailedJobsHistoryLimit:     int32Ptr(1),
				JobTemplate: batchv1.JobTemplateSpec{
					Spec: batchv1.JobSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "curl",
										Image: "curlimages/curl:latest",
										Env: []corev1.EnvVar{
											{
												Name:  "TARGET_URL",
												Value: checker.Spec.TargetURL,
											},
										},
										Command: []string{"sh", "-c"},
										Args: []string{
											"curl -o /dev/null -s -w \"%{http_code}\" ${TARGET_URL}",
										},
									},
								},
								RestartPolicy: corev1.RestartPolicyNever,
							},
						},
					},
				},
			},
		}

		if err := controllerutil.SetControllerReference(checker, newCronJob, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}

		if err := r.Create(ctx, newCronJob); err != nil {
			return ctrl.Result{}, err
		}
		log.Log.Info("CronJob created successfully", "CronJob.Name", newCronJob.Name)
	} else {
		log.Log.Info("CronJob already exists", "CronJob.Name", cronJob.Name)
	}

	return ctrl.Result{}, nil
}

func int32Ptr(i int32) *int32 {
	return &i
}

// SetupWithManager sets up the controller with the Manager.
func (r *CheckerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&checkerv1.Checker{}).
		Complete(r)
}
