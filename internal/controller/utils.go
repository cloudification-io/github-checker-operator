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
var commonLabelKey string = "cloudification.io/checker"

func (r *CheckerReconciler) RenderConfigMap(req *ctrl.Request, checker *checkerv1.Checker) *corev1.ConfigMap {
	thisConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      checker.ObjectMeta.Name,
			Namespace: req.Namespace,
		},
		Data: map[string]string{
			"TARGET_URL": checker.Spec.TargetURL,
		},
	}
	return thisConfigMap
}

func (r *CheckerReconciler) RenderCronJob(req *ctrl.Request, checker *checkerv1.Checker) *batchv1.CronJob {
	thisCronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      checker.ObjectMeta.Name,
			Namespace: req.Namespace,
			Labels: map[string]string{
				commonLabelKey: checker.ObjectMeta.Name,
			},
		},
		Spec: batchv1.CronJobSpec{
			Schedule:                   "* * * * *",
			ConcurrencyPolicy:          batchv1.ReplaceConcurrent,
			SuccessfulJobsHistoryLimit: &cronJobLimitPointer,
			FailedJobsHistoryLimit:     &cronJobLimitPointer,
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								commonLabelKey: checker.ObjectMeta.Name,
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "curl",
									Image: "curlimages/curl:latest",
									EnvFrom: []corev1.EnvFromSource{
										{
											ConfigMapRef: &corev1.ConfigMapEnvSource{
												LocalObjectReference: corev1.LocalObjectReference{
													Name: checker.ObjectMeta.Name,
												},
											},
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
	newConfigMap := r.RenderConfigMap(req, checker)
	if err := r.Create(ctx, newConfigMap); err != nil {
		return ctrl.Result{}, err
	}
	if err := controllerutil.SetControllerReference(checker, newConfigMap, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	log.Log.Info("CronJob created successfully", "ConfigMap.Name", newConfigMap.Name)

	newCronJob := r.RenderCronJob(req, checker)
	if err := r.Create(ctx, newCronJob); err != nil {
		return ctrl.Result{}, err
	}
	if err := controllerutil.SetControllerReference(checker, newCronJob, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	log.Log.Info("CronJob created successfully", "CronJob.Name", newCronJob.Name)

	return ctrl.Result{}, nil
}

func (r *CheckerReconciler) PatchResources(ctx context.Context, req *ctrl.Request, checker *checkerv1.Checker) (ctrl.Result, error) {
	configMap := &corev1.ConfigMap{}
	if err := r.Get(ctx, req.NamespacedName, configMap); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	patchConfigMap := r.RenderConfigMap(req, checker)
	if err := r.Update(ctx, patchConfigMap); err != nil {
		return ctrl.Result{}, err
	}
	log.Log.Info("ConfigMap updated successfully", "ConfigMap.Name", configMap.Name)

	cronJob := &batchv1.CronJob{}
	if err := r.Get(ctx, req.NamespacedName, cronJob); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	patchCronJob := r.RenderCronJob(req, checker)
	if err := r.Update(ctx, patchCronJob); err != nil {
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
