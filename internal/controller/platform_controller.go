package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
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

	tlsMountPath = "/etc/muxcore/tls"
)

// PlatformReconciler reconciles a MuxCorePlatform object.
type PlatformReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=muxcore.media,resources=muxcoreplatforms,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=muxcore.media,resources=muxcoreplatforms/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

func (r *PlatformReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var platform muxcorev1alpha1.MuxCorePlatform
	if err := r.Get(ctx, req.NamespacedName, &platform); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if err := muxcorev1alpha1.ValidateModules(platform.Spec.Modules); err != nil {
		r.recordEvent(&platform, corev1.EventTypeWarning, "ValidationFailed", err.Error())
		meta.SetStatusCondition(&platform.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "ValidationError",
			Message:            err.Error(),
			ObservedGeneration: platform.Generation,
		})
		_ = r.Status().Update(ctx, &platform)
		return ctrl.Result{}, err
	}

	coreImage := platform.Spec.CoreImage
	if coreImage == "" {
		coreImage = muxcorev1alpha1.DefaultCoreImage
	}
	meshAddr := platform.Spec.MeshAddr
	if meshAddr == "" {
		meshAddr = resourceName(platform.Name, muxcorev1alpha1.ReservedModuleName) + ":9090"
	}

	desired := make([]muxcorev1alpha1.ModuleSpec, 0, len(platform.Spec.Modules)+1)
	desired = append(desired, muxcorev1alpha1.ModuleSpec{
		Name:     muxcorev1alpha1.ReservedModuleName,
		Image:    coreImage,
		GrpcPort: 9090,
		Env: map[string]string{
			"MUXCORE_MESH_ADDR": ":9090",
		},
	})
	desired = append(desired, platform.Spec.Modules...)

	ready := int32(0)
	desiredNames := make(map[string]struct{}, len(desired))
	desiredPVCs := make(map[string]struct{})
	for _, mod := range desired {
		resName := resourceName(platform.Name, mod.Name)
		desiredNames[resName] = struct{}{}
		if mod.Volume != nil && mod.Volume.Size != "" {
			desiredPVCs[pvcName(mod, resName)] = struct{}{}
		}
		if err := r.reconcileModule(ctx, &platform, mod, meshAddr, resName); err != nil {
			logger.Error(err, "reconcile module", "module", mod.Name)
			r.recordEvent(&platform, corev1.EventTypeWarning, "ReconcileFailed", fmt.Sprintf("module %s: %v", mod.Name, err))
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
		if err := r.Get(ctx, types.NamespacedName{Namespace: platform.Namespace, Name: resName}, &dep); err == nil {
			if dep.Status.AvailableReplicas >= 1 {
				ready++
			}
		}
	}

	if err := r.pruneModules(ctx, &platform, desiredNames, desiredPVCs); err != nil {
		logger.Error(err, "prune modules")
		r.recordEvent(&platform, corev1.EventTypeWarning, "PruneFailed", err.Error())
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
	platform.Status.DesiredModules = int32(len(desired)) //nolint:gosec // module count is bounded by CRD spec size
	platform.Status.ReadyModules = ready
	readyStatus := metav1.ConditionFalse
	reason := "Progressing"
	msg := fmt.Sprintf("%d/%d modules available", ready, len(desired))
	if ready == int32(len(desired)) && len(desired) > 0 { //nolint:gosec // module count is bounded by CRD spec size
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
	r.recordEvent(&platform, corev1.EventTypeNormal, reason, msg)
	return ctrl.Result{}, nil
}

func resourceName(platformName, moduleName string) string {
	return platformName + "-" + moduleName
}

func (r *PlatformReconciler) recordEvent(platform *muxcorev1alpha1.MuxCorePlatform, eventType, reason, message string) {
	if r.Recorder == nil {
		return
	}
	r.Recorder.Event(platform, eventType, reason, message)
}

func (r *PlatformReconciler) reconcileModule(
	ctx context.Context,
	platform *muxcorev1alpha1.MuxCorePlatform,
	mod muxcorev1alpha1.ModuleSpec,
	meshAddr, resName string,
) error {
	if mod.Name == "" || mod.Image == "" {
		return fmt.Errorf("module name and image are required")
	}

	isCore := mod.Name == muxcorev1alpha1.ReservedModuleName

	labels := map[string]string{
		labelManagedBy:           managedByValue,
		labelPartOf:              partOfValue,
		labelComponent:           mod.Name,
		"muxcore.media/platform": platform.Name,
		"muxcore.media/module":   mod.Name,
	}

	env := make([]corev1.EnvVar, 0, len(mod.Env)+4)
	if isCore {
		if v, ok := mod.Env["MUXCORE_MESH_ADDR"]; ok {
			env = append(env, corev1.EnvVar{Name: "MUXCORE_MESH_ADDR", Value: v})
		} else {
			env = append(env, corev1.EnvVar{Name: "MUXCORE_MESH_ADDR", Value: ":9090"})
		}
	} else {
		env = append(env,
			corev1.EnvVar{Name: "MUXCORE_GRPC_ADDR", Value: meshAddr},
			corev1.EnvVar{Name: "MUXCORE_MODULE_ID", Value: mod.Name},
		)
	}
	if platform.Spec.InsecureDisableTLS {
		env = append(env, corev1.EnvVar{Name: "MUXCORE_INSECURE_DISABLE_TLS", Value: "true"})
	}
	for k, v := range mod.Env {
		if isCore && k == "MUXCORE_MESH_ADDR" {
			continue
		}
		env = append(env, corev1.EnvVar{Name: k, Value: v})
	}

	container := corev1.Container{
		Name:            mod.Name,
		Image:           mod.Image,
		Args:            mod.Args,
		Env:             env,
		ImagePullPolicy: pullPolicy(platform.Spec.ImagePullPolicy),
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: ptr.To(false),
			Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
		},
	}
	if mod.EnvFromSecret != "" {
		container.EnvFrom = []corev1.EnvFromSource{{
			SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: mod.EnvFromSecret}},
		}}
	}

	httpPort, httpSvcPort, grpcPort := modulePorts(mod)
	container.Ports = buildContainerPorts(httpPort, grpcPort)
	if mod.HealthPath != "" && httpPort > 0 {
		probe := &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: mod.HealthPath,
					Port: intstr.FromString("http"),
				},
			},
			InitialDelaySeconds: 3,
			PeriodSeconds:       5,
		}
		container.ReadinessProbe = probe
		container.LivenessProbe = probe
	}

	if res := moduleResources(mod, platform.Spec.DefaultResources); res != nil {
		container.Resources = *res
	}

	podSpec := corev1.PodSpec{
		Containers: []corev1.Container{container},
		SecurityContext: &corev1.PodSecurityContext{
			RunAsNonRoot: ptr.To(true),
		},
	}
	if len(platform.Spec.ImagePullSecrets) > 0 {
		podSpec.ImagePullSecrets = make([]corev1.LocalObjectReference, 0, len(platform.Spec.ImagePullSecrets))
		for _, s := range platform.Spec.ImagePullSecrets {
			podSpec.ImagePullSecrets = append(podSpec.ImagePullSecrets, corev1.LocalObjectReference{Name: s})
		}
	}

	if err := r.applyTLS(platform, &container, &podSpec); err != nil {
		return err
	}
	podSpec.Containers[0] = container

	if mod.Volume != nil && mod.Volume.Size != "" {
		if err := r.reconcilePVC(ctx, platform, mod, resName); err != nil {
			return err
		}
		mountPath := mod.Volume.MountPath
		if mountPath == "" {
			mountPath = "/data"
		}
		claimName := pvcName(mod, resName)
		container.VolumeMounts = []corev1.VolumeMount{{
			Name:      "data",
			MountPath: mountPath,
		}}
		podSpec.Containers[0] = container
		podSpec.Volumes = []corev1.Volume{{
			Name: "data",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: claimName},
			},
		}}
	}

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resName,
			Namespace: platform.Namespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, dep, func() error {
		if err := controllerutil.SetControllerReference(platform, dep, r.Scheme); err != nil {
			return err
		}
		dep.Labels = labels
		dep.Spec.Replicas = ptr.To(int32(1))
		dep.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{labelComponent: mod.Name, "muxcore.media/platform": platform.Name}}
		dep.Spec.Template.Labels = labels
		dep.Spec.Template.Spec = podSpec
		return nil
	})
	if err != nil {
		return fmt.Errorf("deployment %s: %w", resName, err)
	}

	svcPorts := buildServicePorts(httpSvcPort, grpcPort)
	if len(svcPorts) == 0 {
		return nil
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resName,
			Namespace: platform.Namespace,
		},
	}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		if refErr := controllerutil.SetControllerReference(platform, svc, r.Scheme); refErr != nil {
			return refErr
		}
		svc.Labels = labels
		svc.Spec.Selector = map[string]string{labelComponent: mod.Name, "muxcore.media/platform": platform.Name}
		svc.Spec.Ports = svcPorts
		return nil
	})
	if err != nil {
		return fmt.Errorf("service %s: %w", resName, err)
	}
	return nil
}

func pullPolicy(spec string) corev1.PullPolicy {
	switch spec {
	case string(corev1.PullAlways):
		return corev1.PullAlways
	case string(corev1.PullNever):
		return corev1.PullNever
	default:
		return corev1.PullIfNotPresent
	}
}

func modulePorts(mod muxcorev1alpha1.ModuleSpec) (httpPort, httpSvcPort, grpcPort int32) {
	httpPort = mod.HttpPort
	httpSvcPort = mod.HttpServicePort
	grpcPort = mod.GrpcPort
	if httpSvcPort == 0 {
		httpSvcPort = mod.Port
	}
	if httpPort == 0 && httpSvcPort > 0 && grpcPort == 0 {
		httpPort = httpSvcPort
	}
	if httpSvcPort == 0 && httpPort > 0 {
		httpSvcPort = httpPort
	}
	return httpPort, httpSvcPort, grpcPort
}

func buildContainerPorts(httpPort, grpcPort int32) []corev1.ContainerPort {
	ports := make([]corev1.ContainerPort, 0, 2)
	if httpPort > 0 {
		ports = append(ports, corev1.ContainerPort{Name: "http", ContainerPort: httpPort})
	}
	if grpcPort > 0 {
		ports = append(ports, corev1.ContainerPort{Name: "grpc", ContainerPort: grpcPort})
	}
	return ports
}

func buildServicePorts(httpSvcPort, grpcPort int32) []corev1.ServicePort {
	ports := make([]corev1.ServicePort, 0, 2)
	if httpSvcPort > 0 {
		ports = append(ports, corev1.ServicePort{
			Name:       "http",
			Port:       httpSvcPort,
			TargetPort: intstr.FromString("http"),
		})
	}
	if grpcPort > 0 {
		ports = append(ports, corev1.ServicePort{
			Name:       "grpc",
			Port:       grpcPort,
			TargetPort: intstr.FromString("grpc"),
		})
	}
	return ports
}

func moduleResources(mod muxcorev1alpha1.ModuleSpec, defaults *muxcorev1alpha1.ResourceSpec) *corev1.ResourceRequirements {
	spec := mod.Resources
	if spec == nil {
		spec = defaults
	}
	if spec == nil {
		return nil
	}
	out := &corev1.ResourceRequirements{}
	if len(spec.Requests) > 0 {
		out.Requests = make(corev1.ResourceList, len(spec.Requests))
		for k, v := range spec.Requests {
			out.Requests[corev1.ResourceName(k)] = resource.MustParse(v)
		}
	}
	if len(spec.Limits) > 0 {
		out.Limits = make(corev1.ResourceList, len(spec.Limits))
		for k, v := range spec.Limits {
			out.Limits[corev1.ResourceName(k)] = resource.MustParse(v)
		}
	}
	if len(out.Requests) == 0 && len(out.Limits) == 0 {
		return nil
	}
	return out
}

func pvcName(mod muxcorev1alpha1.ModuleSpec, resName string) string {
	if mod.Volume != nil && mod.Volume.ClaimName != "" {
		return mod.Volume.ClaimName
	}
	return resName + "-data"
}

func (r *PlatformReconciler) reconcilePVC(
	ctx context.Context,
	platform *muxcorev1alpha1.MuxCorePlatform,
	mod muxcorev1alpha1.ModuleSpec,
	resName string,
) error {
	name := pvcName(mod, resName)
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: platform.Namespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, pvc, func() error {
		if err := controllerutil.SetControllerReference(platform, pvc, r.Scheme); err != nil {
			return err
		}
		pvc.Labels = map[string]string{
			labelManagedBy:           managedByValue,
			labelPartOf:              partOfValue,
			labelComponent:           mod.Name,
			"muxcore.media/platform": platform.Name,
		}
		if pvc.Spec.AccessModes == nil {
			pvc.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
		}
		if pvc.Spec.Resources.Requests == nil {
			pvc.Spec.Resources.Requests = corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse(mod.Volume.Size),
			}
		}
		if mod.Volume.StorageClassName != "" && pvc.Spec.StorageClassName == nil {
			pvc.Spec.StorageClassName = ptr.To(mod.Volume.StorageClassName)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("pvc %s: %w", name, err)
	}
	return nil
}

func (r *PlatformReconciler) applyTLS(
	platform *muxcorev1alpha1.MuxCorePlatform,
	container *corev1.Container,
	podSpec *corev1.PodSpec,
) error {
	if platform.Spec.InsecureDisableTLS {
		return nil
	}
	secret := platform.Spec.MeshTLSSecret
	if secret == "" {
		return fmt.Errorf("meshTLSSecret is required when insecureDisableTLS is false")
	}
	container.Env = append(container.Env,
		corev1.EnvVar{Name: "MUXCORE_TLS_CERT", Value: tlsMountPath + "/tls.crt"},
		corev1.EnvVar{Name: "MUXCORE_TLS_KEY", Value: tlsMountPath + "/tls.key"},
		corev1.EnvVar{Name: "MUXCORE_TLS_CA", Value: tlsMountPath + "/ca.crt"},
	)
	container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
		Name:      "muxcore-tls",
		MountPath: tlsMountPath,
		ReadOnly:  true,
	})
	podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
		Name: "muxcore-tls",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{SecretName: secret},
		},
	})
	return nil
}

func (r *PlatformReconciler) pruneModules(
	ctx context.Context,
	platform *muxcorev1alpha1.MuxCorePlatform,
	desired map[string]struct{},
	desiredPVCs map[string]struct{},
) error {
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
		r.recordEvent(platform, corev1.EventTypeNormal, "PrunedDeployment", dep.Name)
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
		r.recordEvent(platform, corev1.EventTypeNormal, "PrunedService", svc.Name)
	}

	var pvcs corev1.PersistentVolumeClaimList
	if err := r.List(ctx, &pvcs, client.InNamespace(platform.Namespace), labels); err != nil {
		return fmt.Errorf("list pvcs: %w", err)
	}
	for _, pvc := range pvcs.Items {
		if _, ok := desiredPVCs[pvc.Name]; ok {
			continue
		}
		if err := r.Delete(ctx, &pvc); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete pvc %s: %w", pvc.Name, err)
		}
		r.recordEvent(platform, corev1.EventTypeNormal, "PrunedPVC", pvc.Name)
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PlatformReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("muxcore-platform-controller")
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&muxcorev1alpha1.MuxCorePlatform{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Complete(r)
}
