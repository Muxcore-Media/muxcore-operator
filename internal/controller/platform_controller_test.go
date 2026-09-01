package controller_test

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	muxcorev1alpha1 "github.com/Muxcore-Media/muxcore-operator/api/v1alpha1"
	"github.com/Muxcore-Media/muxcore-operator/internal/controller"
)

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("core scheme: %v", err)
	}
	if err := muxcorev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("muxcore scheme: %v", err)
	}
	return scheme
}

func testReconciler(t *testing.T, scheme *runtime.Scheme, objs ...client.Object) *controller.PlatformReconciler {
	t.Helper()
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&muxcorev1alpha1.MuxCorePlatform{}).
		WithObjects(objs...).Build()
	return &controller.PlatformReconciler{
		Client:   c,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(32),
	}
}

func TestPlatformReconciler_CreatesCoreAndModules(t *testing.T) {
	scheme := testScheme(t)
	platform := &muxcorev1alpha1.MuxCorePlatform{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "demo",
			Namespace:  "muxcore",
			Generation: 1,
		},
		Spec: muxcorev1alpha1.MuxCorePlatformSpec{
			InsecureDisableTLS: true,
			Modules: []muxcorev1alpha1.ModuleSpec{{
				Name:            "api-rest",
				Image:           "git.zem.systems/muxcore/api-rest:v0.1.6",
				HttpPort:        8080,
				HttpServicePort: 18080,
				GrpcPort:        9400,
				HealthPath:      "/api/v1/health",
				Env: map[string]string{
					"API_REST_HTTP_ADDR": ":8080",
					"API_REST_GRPC_ADDR": ":9400",
				},
			}},
		},
	}

	r := testReconciler(t, scheme, platform)
	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "demo", Namespace: "muxcore"},
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	var core appsv1.Deployment
	if err := r.Get(context.Background(), types.NamespacedName{Namespace: "muxcore", Name: "demo-muxcored"}, &core); err != nil {
		t.Fatalf("get muxcored deployment: %v", err)
	}
	if core.Spec.Template.Spec.Containers[0].Image != muxcorev1alpha1.DefaultCoreImage {
		t.Fatalf("core image = %q want default", core.Spec.Template.Spec.Containers[0].Image)
	}
	meshEnv := envMap(core.Spec.Template.Spec.Containers[0].Env)
	if meshEnv["MUXCORE_MESH_ADDR"] != ":9090" {
		t.Fatalf("muxcored MUXCORE_MESH_ADDR = %q", meshEnv["MUXCORE_MESH_ADDR"])
	}
	if _, ok := meshEnv["MUXCORE_GRPC_ADDR"]; ok {
		t.Fatal("muxcored must not set MUXCORE_GRPC_ADDR")
	}

	var api appsv1.Deployment
	if err := r.Get(context.Background(), types.NamespacedName{Namespace: "muxcore", Name: "demo-api-rest"}, &api); err != nil {
		t.Fatalf("get api-rest deployment: %v", err)
	}
	apiEnv := envMap(api.Spec.Template.Spec.Containers[0].Env)
	if apiEnv["MUXCORE_GRPC_ADDR"] != "demo-muxcored:9090" {
		t.Fatalf("MUXCORE_GRPC_ADDR = %q", apiEnv["MUXCORE_GRPC_ADDR"])
	}
	if apiEnv["MUXCORE_MODULE_ID"] != "api-rest" {
		t.Fatalf("MUXCORE_MODULE_ID = %q", apiEnv["MUXCORE_MODULE_ID"])
	}
	if apiEnv["MUXCORE_INSECURE_DISABLE_TLS"] != "true" {
		t.Fatalf("missing insecure TLS env")
	}

	var svc corev1.Service
	if err := r.Get(context.Background(), types.NamespacedName{Namespace: "muxcore", Name: "demo-muxcored"}, &svc); err != nil {
		t.Fatalf("get muxcored service: %v", err)
	}
	if len(svc.Spec.Ports) != 1 || svc.Spec.Ports[0].Name != "grpc" {
		t.Fatalf("muxcored service ports: %+v", svc.Spec.Ports)
	}

	var updated muxcorev1alpha1.MuxCorePlatform
	if err := r.Get(context.Background(), types.NamespacedName{Namespace: "muxcore", Name: "demo"}, &updated); err != nil {
		t.Fatalf("get platform: %v", err)
	}
	if updated.Status.DesiredModules != 2 {
		t.Fatalf("DesiredModules=%d want 2", updated.Status.DesiredModules)
	}
}

func TestPlatformReconciler_DefaultCoreImageAndMeshAddr(t *testing.T) {
	scheme := testScheme(t)
	platform := &muxcorev1alpha1.MuxCorePlatform{
		ObjectMeta: metav1.ObjectMeta{Name: "lab", Namespace: "muxcore", Generation: 1},
		Spec:       muxcorev1alpha1.MuxCorePlatformSpec{InsecureDisableTLS: true},
	}
	r := testReconciler(t, scheme, platform)
	if _, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "lab", Namespace: "muxcore"},
	}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	var sidecar appsv1.Deployment
	if err := r.Get(context.Background(), types.NamespacedName{Namespace: "muxcore", Name: "lab-muxcored"}, &sidecar); err != nil {
		t.Fatalf("get core: %v", err)
	}
	if sidecar.Spec.Template.Spec.Containers[0].Image != muxcorev1alpha1.DefaultCoreImage {
		t.Fatalf("image = %q", sidecar.Spec.Template.Spec.Containers[0].Image)
	}
}

func TestPlatformReconciler_EnvFromSecret(t *testing.T) {
	scheme := testScheme(t)
	platform := &muxcorev1alpha1.MuxCorePlatform{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "muxcore", Generation: 1},
		Spec: muxcorev1alpha1.MuxCorePlatformSpec{
			InsecureDisableTLS: true,
			Modules: []muxcorev1alpha1.ModuleSpec{{
				Name:          "auth-local",
				Image:         "git.zem.systems/muxcore/auth-local:v0.1.5",
				GrpcPort:      9403,
				HttpPort:      9401,
				EnvFromSecret: "muxcore-auth",
			}},
		},
	}
	r := testReconciler(t, scheme, platform)
	if _, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "demo", Namespace: "muxcore"},
	}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	var dep appsv1.Deployment
	if err := r.Get(context.Background(), types.NamespacedName{Namespace: "muxcore", Name: "demo-auth-local"}, &dep); err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	envFrom := dep.Spec.Template.Spec.Containers[0].EnvFrom
	if len(envFrom) != 1 || envFrom[0].SecretRef == nil || envFrom[0].SecretRef.Name != "muxcore-auth" {
		t.Fatalf("envFrom = %+v", envFrom)
	}
}

func TestPlatformReconciler_RequiresMeshTLSSecret(t *testing.T) {
	scheme := testScheme(t)
	platform := &muxcorev1alpha1.MuxCorePlatform{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "muxcore", Generation: 1},
		Spec: muxcorev1alpha1.MuxCorePlatformSpec{
			InsecureDisableTLS: false,
		},
	}
	r := testReconciler(t, scheme, platform)
	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "demo", Namespace: "muxcore"},
	})
	if err == nil || !strings.Contains(err.Error(), "meshTLSSecret") {
		t.Fatalf("expected meshTLSSecret error, got %v", err)
	}
}

func TestPlatformReconciler_InjectsMeshTLS(t *testing.T) {
	scheme := testScheme(t)
	platform := &muxcorev1alpha1.MuxCorePlatform{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "muxcore", Generation: 1},
		Spec: muxcorev1alpha1.MuxCorePlatformSpec{
			InsecureDisableTLS: false,
			MeshTLSSecret:      "muxcore-mesh-tls",
		},
	}
	r := testReconciler(t, scheme, platform)
	if _, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "demo", Namespace: "muxcore"},
	}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	var dep appsv1.Deployment
	if err := r.Get(context.Background(), types.NamespacedName{Namespace: "muxcore", Name: "demo-muxcored"}, &dep); err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	c := dep.Spec.Template.Spec.Containers[0]
	env := envMap(c.Env)
	for _, k := range []string{"MUXCORE_TLS_CERT", "MUXCORE_TLS_KEY", "MUXCORE_TLS_CA"} {
		if env[k] == "" {
			t.Fatalf("missing %s", k)
		}
	}
	if _, ok := env["MUXCORE_INSECURE_DISABLE_TLS"]; ok {
		t.Fatal("should not set insecure when TLS enabled")
	}
	foundVol := false
	for _, v := range dep.Spec.Template.Spec.Volumes {
		if v.Name == "muxcore-tls" && v.Secret != nil && v.Secret.SecretName == "muxcore-mesh-tls" {
			foundVol = true
		}
	}
	if !foundVol {
		t.Fatal("missing tls volume")
	}
}

func TestPlatformReconciler_ReadyWhenAvailableReplicas(t *testing.T) {
	scheme := testScheme(t)
	platform := &muxcorev1alpha1.MuxCorePlatform{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "muxcore", Generation: 1},
		Spec:       muxcorev1alpha1.MuxCorePlatformSpec{InsecureDisableTLS: true},
	}
	r := testReconciler(t, scheme, platform)
	if _, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "demo", Namespace: "muxcore"},
	}); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	var dep appsv1.Deployment
	if err := r.Get(context.Background(), types.NamespacedName{Namespace: "muxcore", Name: "demo-muxcored"}, &dep); err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	dep.Status.AvailableReplicas = 1
	if err := r.Status().Update(context.Background(), &dep); err != nil {
		t.Fatalf("update deployment status: %v", err)
	}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "demo", Namespace: "muxcore"},
	}); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	var updated muxcorev1alpha1.MuxCorePlatform
	if err := r.Get(context.Background(), types.NamespacedName{Namespace: "muxcore", Name: "demo"}, &updated); err != nil {
		t.Fatalf("get platform: %v", err)
	}
	if updated.Status.ReadyModules != 1 {
		t.Fatalf("ReadyModules=%d want 1", updated.Status.ReadyModules)
	}
	cond := metaReady(updated.Status.Conditions)
	if cond == nil || cond.Status != metav1.ConditionTrue {
		t.Fatalf("Ready condition = %+v", cond)
	}
}

func TestPlatformReconciler_PrunesRemovedModules(t *testing.T) {
	scheme := testScheme(t)
	platform := &muxcorev1alpha1.MuxCorePlatform{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "muxcore", Generation: 2},
		Spec:       muxcorev1alpha1.MuxCorePlatformSpec{InsecureDisableTLS: true},
	}
	labels := map[string]string{
		"app.kubernetes.io/managed-by": "muxcore-operator",
		"muxcore.media/platform":       "demo",
		"app.kubernetes.io/part-of":    "muxcore",
		"app.kubernetes.io/component":  "api-rest",
	}
	staleDep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "demo-api-rest", Namespace: "muxcore", Labels: labels},
	}
	staleSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "demo-api-rest", Namespace: "muxcore", Labels: labels},
	}
	r := testReconciler(t, scheme, platform, staleDep, staleSvc)
	if _, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "demo", Namespace: "muxcore"},
	}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	var gone appsv1.Deployment
	if err := r.Get(context.Background(), types.NamespacedName{Namespace: "muxcore", Name: "demo-api-rest"}, &gone); err == nil {
		t.Fatal("expected stale deployment to be pruned")
	}
	var goneSvc corev1.Service
	if err := r.Get(context.Background(), types.NamespacedName{Namespace: "muxcore", Name: "demo-api-rest"}, &goneSvc); err == nil {
		t.Fatal("expected stale service to be pruned")
	}
}

func TestPlatformReconciler_PrunesPVC(t *testing.T) {
	scheme := testScheme(t)
	platform := &muxcorev1alpha1.MuxCorePlatform{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "muxcore", Generation: 2},
		Spec:       muxcorev1alpha1.MuxCorePlatformSpec{InsecureDisableTLS: true},
	}
	labels := map[string]string{
		"app.kubernetes.io/managed-by": "muxcore-operator",
		"muxcore.media/platform":       "demo",
	}
	stalePVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "demo-auth-local-data", Namespace: "muxcore", Labels: labels},
	}
	r := testReconciler(t, scheme, platform, stalePVC)
	if _, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "demo", Namespace: "muxcore"},
	}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	var gone corev1.PersistentVolumeClaim
	if err := r.Get(context.Background(), types.NamespacedName{Namespace: "muxcore", Name: "demo-auth-local-data"}, &gone); err == nil {
		t.Fatal("expected stale pvc to be pruned")
	}
}

func TestPlatformReconciler_CreatesPVC(t *testing.T) {
	scheme := testScheme(t)
	platform := &muxcorev1alpha1.MuxCorePlatform{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "muxcore", Generation: 1},
		Spec: muxcorev1alpha1.MuxCorePlatformSpec{
			InsecureDisableTLS: true,
			Modules: []muxcorev1alpha1.ModuleSpec{{
				Name:  "secrets-file",
				Image: "git.zem.systems/muxcore/secrets-file:v0.1.6",
				GrpcPort: 9550,
				Volume: &muxcorev1alpha1.VolumeSpec{
					Size: "1Gi",
				},
			}},
		},
	}
	r := testReconciler(t, scheme, platform)
	if _, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "demo", Namespace: "muxcore"},
	}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	var pvc corev1.PersistentVolumeClaim
	if err := r.Get(context.Background(), types.NamespacedName{Namespace: "muxcore", Name: "demo-secrets-file-data"}, &pvc); err != nil {
		t.Fatalf("get pvc: %v", err)
	}
	if got := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; got.Cmp(resource.MustParse("1Gi")) != 0 {
		t.Fatalf("storage request = %v want 1Gi", got)
	}
}

func TestPlatformReconciler_EmptyNameOrImageErrors(t *testing.T) {
	scheme := testScheme(t)
	platform := &muxcorev1alpha1.MuxCorePlatform{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "muxcore", Generation: 1},
		Spec: muxcorev1alpha1.MuxCorePlatformSpec{
			InsecureDisableTLS: true,
			Modules: []muxcorev1alpha1.ModuleSpec{{
				Name:  "bad",
				Image: "",
			}},
		},
	}
	r := testReconciler(t, scheme, platform)
	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "demo", Namespace: "muxcore"},
	})
	if err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("expected required error, got %v", err)
	}
}

func TestPlatformReconciler_ReservedModuleNameRejected(t *testing.T) {
	scheme := testScheme(t)
	platform := &muxcorev1alpha1.MuxCorePlatform{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "muxcore", Generation: 1},
		Spec: muxcorev1alpha1.MuxCorePlatformSpec{
			InsecureDisableTLS: true,
			Modules: []muxcorev1alpha1.ModuleSpec{{
				Name:  "muxcored",
				Image: "example.invalid/muxcored:latest",
			}},
		},
	}
	r := testReconciler(t, scheme, platform)
	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "demo", Namespace: "muxcore"},
	})
	if err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("expected reserved error, got %v", err)
	}
}

func TestValidateModules_DuplicateName(t *testing.T) {
	err := muxcorev1alpha1.ValidateModules([]muxcorev1alpha1.ModuleSpec{
		{Name: "api-rest", Image: "x"},
		{Name: "api-rest", Image: "y"},
	})
	if err == nil {
		t.Fatal("expected duplicate error")
	}
}

func envMap(vars []corev1.EnvVar) map[string]string {
	out := make(map[string]string, len(vars))
	for _, e := range vars {
		out[e.Name] = e.Value
	}
	return out
}

func metaReady(conds []metav1.Condition) *metav1.Condition {
	for i := range conds {
		if conds[i].Type == "Ready" {
			return &conds[i]
		}
	}
	return nil
}
