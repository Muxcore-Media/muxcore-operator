package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	muxcorev1alpha1 "github.com/Muxcore-Media/muxcore-operator/api/v1alpha1"
)

const (
	labelManagedBy = "app.kubernetes.io/managed-by"
	labelPartOf    = "app.kubernetes.io/part-of"
	labelComponent = "app.kubernetes.io/component"
	managedByValue = "muxcore-operator"
	partOfValue    = "muxcore"
)

// PlatformReconciler reconciles a MuxCorePlatform object.
type PlatformReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=muxcore.media,resources=muxcoreplatforms,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=muxcore.media,resources=muxcoreplatforms/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

func (r *PlatformReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var platform muxcorev1alpha1.MuxCorePlatform
	if err := r.Get(ctx, req.NamespacedName, &platform); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	coreImage := platform.Spec.CoreImage
	if coreImage == "" {
		coreImage = "ghcr.io/muxcore-media/muxcored:v0.5.4"
	}
	meshAddr := platform.Spec.MeshAddr
	if meshAddr == "" {
		meshAddr = "muxcored:9090"
	}

	desired := make([]muxcorev1alpha1.ModuleSpec, 0, len(platform.Spec.Modules)+1)
	desired = append(desired, muxcorev1alpha1.ModuleSpec{
		Name:  "muxcored",
		Image: coreImage,
		Port:  9090,
		Env: map[string]string{
			"MUXCORE_MESH_ADDR": ":9090",
		},
	})
	desired = append(desired, platform.Spec.Modules...)

	ready := int32(0)
	desiredNames := make(map[string]struct{}, len(desired))
	for _, mod := range desired {
		desiredNames[mod.Name] = struct{}{}
		if err := r.reconcileModule(ctx, &platform, mod, meshAddr); err != nil {
			logger.Error(err, "reconcile module", "module", mod.Name)
			meta.SetStatusCondition(&platform.Status.Conditions, metav1.Condition{
				Type:               "Ready",
				Status:             metav1.ConditionFalse,
				Reason:             "ReconcileError",
				Message:            err.Error(),
				ObservedGeneration: platform.Generation,
			})
			_ = r.Status().Update(ctx, &platform)
			return ctrl.Result{}, err
		}
		var dep appsv1.Deployment
		if err := r.Get(ctx, types.NamespacedName{Namespace: platform.Namespace, Name: mod.Name}, &dep); err == nil {
			if dep.Status.AvailableReplicas > 0 {
				ready++
			}
		}
	}

	if err := r.pruneModules(ctx, &platform, desiredNames); err != nil {
		logger.Error(err, "prune modules")
		meta.SetStatusCondition(&platform.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "PruneError",
			Message:            err.Error(),
			ObservedGeneration: platform.Generation,
		})
		_ = r.Status().Update(ctx, &platform)
		return ctrl.Result{}, err
	}

	platform.Status.ObservedGeneration = platform.Generation
	platform.Status.DesiredModules = int32(len(desired))
	platform.Status.ReadyModules = ready
	readyStatus := metav1.ConditionFalse
	reason := "Progressing"
	msg := fmt.Sprintf("%d/%d modules available", ready, len(desired))
	if ready == int32(len(desired)) && len(desired) > 0 {
		readyStatus = metav1.ConditionTrue
		reason = "Available"
	}
	meta.SetStatusCondition(&platform.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             readyStatus,
		Reason:             reason,
		Message:            msg,
		ObservedGeneration: platform.Generation,
	})
	if err := r.Status().Update(ctx, &platform); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *PlatformReconciler) reconcileModule(ctx context.Context, platform *muxcorev1alpha1.MuxCorePlatform, mod muxcorev1alpha1.ModuleSpec, meshAddr string) error {
	if mod.Name == "" || mod.Image == "" {
		return fmt.Errorf("module name and image are required")
	}

	labels := map[string]string{
		labelManagedBy: managedByValue,
		labelPartOf:    partOfValue,
		labelComponent: mod.Name,
		"muxcore.media/platform": platform.Name,
	}

	env := []corev1.EnvVar{
		{Name: "MUXCORE_MESH_ADDR", Value: meshAddr},
	}
	if platform.Spec.InsecureDisableTLS {
		env = append(env, corev1.EnvVar{Name: "MUXCORE_INSECURE_DISABLE_TLS", Value: "true"})
	}
	for k, v := range mod.Env {
		env = append(env, corev1.EnvVar{Name: k, Value: v})
	}

	container := corev1.Container{
		Name:  mod.Name,
		Image: mod.Image,
		Args:  mod.Args,
		Env:   env,
	}
	if mod.EnvFromSecret != "" {
		container.EnvFrom = []corev1.EnvFromSource{{
			SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: mod.EnvFromSecret}},
		}}
	}
	port := mod.Port
	if port == 0 {
		port = 8080
	}
	container.Ports = []corev1.ContainerPort{{Name: "primary", ContainerPort: port}}

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mod.Name,
			Namespace: platform.Namespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, dep, func() error {
		if err := controllerutil.SetControllerReference(platform, dep, r.Scheme); err != nil {
			return err
		}
		dep.Labels = labels
		dep.Spec.Replicas = ptr.To(int32(1))
		dep.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{labelComponent: mod.Name}}
		dep.Spec.Template.ObjectMeta.Labels = labels
		dep.Spec.Template.Spec.Containers = []corev1.Container{container}
		return nil
	})
	if err != nil {
		return fmt.Errorf("deployment %s: %w", mod.Name, err)
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mod.Name,
			Namespace: platform.Namespace,
		},
	}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		if err := controllerutil.SetControllerReference(platform, svc, r.Scheme); err != nil {
			return err
		}
		svc.Labels = labels
		svc.Spec.Selector = map[string]string{labelComponent: mod.Name}
		svc.Spec.Ports = []corev1.ServicePort{{
			Name:       "primary",
			Port:       port,
			TargetPort: intstr.FromInt32(port),
		}}
		return nil
	})
	if err != nil {
		return fmt.Errorf("service %s: %w", mod.Name, err)
	}
	return nil
}

func (r *PlatformReconciler) pruneModules(ctx context.Context, platform *muxcorev1alpha1.MuxCorePlatform, desired map[string]struct{}) error {
	labels := client.MatchingLabels{
		labelManagedBy:           managedByValue,
		"muxcore.media/platform": platform.Name,
	}

	var deps appsv1.DeploymentList
	if err := r.List(ctx, &deps, client.InNamespace(platform.Namespace), labels); err != nil {
		return fmt.Errorf("list deployments: %w", err)
	}
	for _, dep := range deps.Items {
		if _, ok := desired[dep.Name]; ok {
			continue
		}
		if err := r.Delete(ctx, &dep); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete deployment %s: %w", dep.Name, err)
		}
	}

	var svcs corev1.ServiceList
	if err := r.List(ctx, &svcs, client.InNamespace(platform.Namespace), labels); err != nil {
		return fmt.Errorf("list services: %w", err)
	}
	for _, svc := range svcs.Items {
		if _, ok := desired[svc.Name]; ok {
			continue
		}
		if err := r.Delete(ctx, &svc); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete service %s: %w", svc.Name, err)
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PlatformReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&muxcorev1alpha1.MuxCorePlatform{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
