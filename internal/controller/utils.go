package controller

import (
	"context"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	checkerv1 "github.com/cloudification-io/github-checker-operator/api/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var cronJobLimitPointer int32 = 1

func (r *CheckerReconciler) RenderCronJob(req *ctrl.Request, checker *checkerv1.Checker) *batchv1.CronJob {
	thisCronJob := &batchv1.CronJob{
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
	return thisCronJob
}

func (r *CheckerReconciler) CreateResources(ctx context.Context, req *ctrl.Request, checker *checkerv1.Checker) (ctrl.Result, error) {
	newCronJob := r.RenderCronJob(req, checker)

	if err := controllerutil.SetControllerReference(checker, newCronJob, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.Create(ctx, newCronJob); err != nil {
		return ctrl.Result{}, err
	}

	log.Log.Info("CronJob created successfully", "CronJob.Name", newCronJob.Name)

	return ctrl.Result{}, nil
}

func (r *CheckerReconciler) PatchResources(ctx context.Context, req *ctrl.Request, checker *checkerv1.Checker) (ctrl.Result, error) {
	cronJob := &batchv1.CronJob{}
	if err := r.Get(ctx, req.NamespacedName, cronJob); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if cronJob == r.RenderCronJob(req, checker) {
		return ctrl.Result{}, nil
	}

	cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Env[0].Value = checker.Spec.TargetURL
	if err := r.Update(ctx, cronJob); err != nil {
		return ctrl.Result{}, err
	}
	log.Log.Info("CronJob updated successfully", "CronJob.Name", cronJob.Name)

	return ctrl.Result{Requeue: true, RequeueAfter: 10 * time.Second}, nil
}

func (r *CheckerReconciler) UpdateStatus(ctx context.Context, req ctrl.Request, checker *checkerv1.Checker) (ctrl.Result, error) {
	jobList := &batchv1.JobList{}
	if err := r.List(ctx, jobList, client.InNamespace(req.Namespace)); err != nil {
		return ctrl.Result{}, err
	}

	for _, job := range jobList.Items {
		if job.Status.Succeeded > 0 {
			// log.Log.Info("Job completed successfully", "Job.Name", job.Name)
			checker.Status.TargetStatus = "Ok"
		} else {
			// log.Log.Info("Job failed", "Job.Name", job.Name)
			checker.Status.TargetStatus = "Not Ok"
		}

		podList := &corev1.PodList{}
		if err := r.List(ctx, podList, client.InNamespace(req.Namespace)); err != nil {
			// log.Log.Error(err, "Unable to list Pods for Job", "Job.Name", job.Name)
			continue
		}

		for _, pod := range podList.Items {
			// log.Log.Info("Fetching logs for Pod", "Pod.Name", pod.Name)
			podLogs, err := r.getPodLogs(ctx, pod)
			if err != nil {
				// log.Log.Error(err, "Unable to get logs for Pod", "Pod.Name", pod.Name)
				checker.Status.TargetStatus = "Unknown"
			} else {
				log.Log.Info("Pod Logs", "Pod.Name", pod.Name, "Logs", podLogs)
				checker.Status.TargetStatus = podLogs
			}
		}
	}

	if err := r.Status().Update(ctx, checker); err != nil {
		// log.Log.Error(err, "unable to update CronJob status")
		return ctrl.Result{}, err
	}

	log.Log.Info("Status updated successfully", "Checker.Name", checker.Name)

	return ctrl.Result{}, nil
}

func (r *CheckerReconciler) getPodLogs(ctx context.Context, pod corev1.Pod) (string, error) {
	return "200", nil
}
