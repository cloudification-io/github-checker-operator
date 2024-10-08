package controller

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
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
var unknownStatus string = "Unknown"
var httpStatusCodeRegex = regexp.MustCompile(`\b(2\d{2}|3\d{2}|4\d{2}|5\d{2})\b`)

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
									Image: "curlimages/curl:8.10.1",
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

func (r *CheckerReconciler) CheckIfResourceExists(ctx context.Context, req *ctrl.Request, resource client.Object) (bool, error) {
	err := r.Get(ctx, req.NamespacedName, resource)
	if err == nil {
		return true, nil
	}
	if client.IgnoreNotFound(err) == nil {
		return false, nil
	}
	return false, err
}

func (r *CheckerReconciler) CreateConfigMap(ctx context.Context, req *ctrl.Request, checker *checkerv1.Checker) error {
	newConfigMap := r.RenderConfigMap(req, checker)

	exists, err := r.CheckIfResourceExists(ctx, req, newConfigMap)
	if err != nil {
		return fmt.Errorf("failed to check ConfigMap existence: %w", err)
	}
	if exists {
		log.Log.Info("ConfigMap already exists, skipping creation", "ConfigMap.Name", newConfigMap.Name)
		err := r.PatchConfigMap(ctx, req, checker)
		if err != nil {
			return err
		}
		return nil
	}

	if err := controllerutil.SetControllerReference(checker, newConfigMap, r.Scheme); err != nil {
		return err
	}
	if err := r.Create(ctx, newConfigMap); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return err
		}
	}
	log.Log.Info("ConfigMap created successfully", "ConfigMap.Name", newConfigMap.Name)

	return nil
}

func (r *CheckerReconciler) CreateCronJob(ctx context.Context, req *ctrl.Request, checker *checkerv1.Checker) error {
	newCronJob := r.RenderCronJob(req, checker)

	exists, err := r.CheckIfResourceExists(ctx, req, newCronJob)
	if err != nil {
		return fmt.Errorf("failed to check CronJob existence: %w", err)
	}
	if exists {
		log.Log.Info("CronJob already exists, skipping creation", "CronJob.Name", newCronJob.Name)
		err := r.PatchCronJob(ctx, req, checker)
		if err != nil {
			return err
		}
		return nil
	}

	if err := controllerutil.SetControllerReference(checker, newCronJob, r.Scheme); err != nil {
		return err
	}
	if err := r.Create(ctx, newCronJob); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return err
		}
	}
	log.Log.Info("CronJob created successfully", "CronJob.Name", newCronJob.Name)

	return nil
}

func (r *CheckerReconciler) ReconcileResources(ctx context.Context, req *ctrl.Request, checker *checkerv1.Checker) error {
	if err := r.CreateConfigMap(ctx, req, checker); err != nil {
		return err
	}

	if err := r.CreateCronJob(ctx, req, checker); err != nil {
		return err
	}

	return nil
}

func (r *CheckerReconciler) PatchCronJob(ctx context.Context, req *ctrl.Request, checker *checkerv1.Checker) error {
	cronJob := &batchv1.CronJob{}
	if err := r.Get(ctx, req.NamespacedName, cronJob); err != nil {
		return client.IgnoreNotFound(err)
	}
	patchCronJob := r.RenderCronJob(req, checker)
	if err := controllerutil.SetControllerReference(checker, patchCronJob, r.Scheme); err != nil {
		return err
	}
	if err := r.Update(ctx, patchCronJob); err != nil {
		return err
	}
	log.Log.Info("CronJob updated successfully", "CronJob.Name", cronJob.Name)

	return nil
}

func (r *CheckerReconciler) PatchConfigMap(ctx context.Context, req *ctrl.Request, checker *checkerv1.Checker) error {
	configMap := &corev1.ConfigMap{}
	if err := r.Get(ctx, req.NamespacedName, configMap); err != nil {
		return client.IgnoreNotFound(err)
	}
	patchConfigMap := r.RenderConfigMap(req, checker)
	if err := controllerutil.SetControllerReference(checker, patchConfigMap, r.Scheme); err != nil {
		return err
	}
	if err := r.Update(ctx, patchConfigMap); err != nil {
		return err
	}
	log.Log.Info("ConfigMap updated successfully", "ConfigMap.Name", configMap.Name)

	return nil
}

func (r *CheckerReconciler) UpdateStatus(ctx context.Context, req *ctrl.Request, checker *checkerv1.Checker) error {
	status, err := r.getPodLogs(ctx, checker)
	if err != nil {
		log.Log.Error(err, "Could not get logs from pod")
	}

	if err := r.SetStatus(ctx, checker, status); err != nil {
		log.Log.Error(err, "Unable to update Checker status", "checker.Name", checker.Name)
		return err
	}

	log.Log.Info("Status updated successfully", "Checker.Name", checker.Name)
	return nil
}

func (r *CheckerReconciler) SetStatus(ctx context.Context, checker *checkerv1.Checker, status string) error {
	if statusMatches := httpStatusCodeRegex.FindString(status); statusMatches != "" {
		checker.Status.TargetStatus = statusMatches
	} else {
		checker.Status.TargetStatus = unknownStatus
	}

	if err := r.Status().Update(ctx, checker); err != nil {
		log.Log.Error(err, "Unable to update Checker status", "checker.Name", checker.Name)
		return err
	}

	return nil
}

func (r *CheckerReconciler) getPodLogs(ctx context.Context, checker *checkerv1.Checker) (string, error) {
	podList, err := r.Clientset.CoreV1().Pods(checker.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%v=%s", commonLabelKey, checker.ObjectMeta.Name),
	})
	if err != nil || len(podList.Items) < 1 {
		return unknownStatus, err
	}

	latestPod, err := r.findLatestPod(podList.Items)
	if err != nil {
		return unknownStatus, err
	}

	podCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	err = r.waitForPodSucceeded(podCtx, checker.Namespace, latestPod.Name)
	if err != nil {
		return unknownStatus, err
	}

	log.Log.Info("Looking up logs from found latest pod", "latestPod.Name", latestPod.Name)
	req := r.Clientset.CoreV1().Pods(checker.Namespace).GetLogs(latestPod.Name, &corev1.PodLogOptions{})

	podLogs, err := req.Stream(ctx)
	if err != nil {
		return unknownStatus, err
	}
	defer podLogs.Close()

	buf := new(bytes.Buffer)
	if _, err = buf.ReadFrom(podLogs); err != nil {
		return unknownStatus, err
	}

	return buf.String(), nil
}

func (r *CheckerReconciler) waitForPodSucceeded(ctx context.Context, namespace, podName string) error {
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context deadline exceeded waiting for pod %s to reach 'Succeeded' phase", podName)

		default:
			pod, err := r.Clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("error retrieving pod %s: %v", podName, err)
			}

			if pod.Status.Phase == corev1.PodSucceeded {
				log.Log.Info("Pod has reached 'Succeeded' phase", "PodName", podName)
				return nil
			}

			time.Sleep(time.Second)
		}
	}
}

func (r *CheckerReconciler) findLatestPod(pods []corev1.Pod) (*corev1.Pod, error) {
	var latestPod *corev1.Pod

	for _, pod := range pods {
		if latestPod == nil || pod.CreationTimestamp.After(latestPod.CreationTimestamp.Time) {
			latestPod = &pod
		}
	}

	if latestPod == nil {
		return nil, fmt.Errorf("no pods found")
	}

	return latestPod, nil
}
