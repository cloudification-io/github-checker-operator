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

var cronJobLimitPointer int32 = 0

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
	log.Log.Info("Trigger reconcile:", "req.NamespacedName", req.NamespacedName)

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
			},
			Spec: batchv1.CronJobSpec{
				Schedule:                   "* * * * *",
				ConcurrencyPolicy:          batchv1.ReplaceConcurrent,
				SuccessfulJobsHistoryLimit: &cronJobLimitPointer,
				FailedJobsHistoryLimit:     &cronJobLimitPointer,
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

		return ctrl.Result{Requeue: true, RequeueAfter: 10 * time.Second}, nil
	} else {
		if cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Env[0].Value != checker.Spec.TargetURL {
			cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Env[0].Value = checker.Spec.TargetURL
			if err := r.Update(ctx, cronJob); err != nil {
				return ctrl.Result{}, err
			}
			log.Log.Info("CronJob updated successfully", "CronJob.Name", cronJob.Name)

			return ctrl.Result{Requeue: true, RequeueAfter: 10 * time.Second}, nil
		}
	}

	jobList := &batchv1.JobList{}
	if err := r.List(ctx, jobList, client.InNamespace(req.Namespace)); err != nil {
		return ctrl.Result{}, err
	}

	for _, job := range jobList.Items {
		if job.Status.Succeeded > 0 {
			log.Log.Info("Job completed successfully", "Job.Name", job.Name)
			checker.Status.TargetStatus = "Ok"
		} else {
			log.Log.Info("Job failed", "Job.Name", job.Name)
			checker.Status.TargetStatus = "Not Ok"
		}

		podList := &corev1.PodList{}
		if err := r.List(ctx, podList, client.InNamespace(req.Namespace)); err != nil {
			log.Log.Error(err, "Unable to list Pods for Job", "Job.Name", job.Name)
			continue
		}

		for _, pod := range podList.Items {
			log.Log.Info("Fetching logs for Pod", "Pod.Name", pod.Name)
			podLogs, err := r.getPodLogs(ctx, pod)
			if err != nil {
				log.Log.Error(err, "Unable to get logs for Pod", "Pod.Name", pod.Name)
				checker.Status.TargetStatus = "Unknown"
			} else {
				log.Log.Info("Pod Logs", "Pod.Name", pod.Name, "Logs", podLogs)
				checker.Status.TargetStatus = podLogs
			}
		}
	}

	if err := r.Status().Update(ctx, checker); err != nil {
		log.Log.Error(err, "unable to update CronJob status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *CheckerReconciler) getPodLogs(ctx context.Context, pod corev1.Pod) (string, error) {
	return "200", nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CheckerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&checkerv1.Checker{}).
		Owns(&batchv1.CronJob{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
