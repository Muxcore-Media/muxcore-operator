package controller_test

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	muxcorev1alpha1 "github.com/Muxcore-Media/muxcore-operator/api/v1alpha1"
	"github.com/Muxcore-Media/muxcore-operator/internal/controller"
)

func TestPlatformReconciler_CreatesCoreAndModules(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = muxcorev1alpha1.AddToScheme(scheme)

	platform := &muxcorev1alpha1.MuxCorePlatform{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "demo",
			Namespace:  "muxcore",
			Generation: 1,
		},
		Spec: muxcorev1alpha1.MuxCorePlatformSpec{
			InsecureDisableTLS: true,
			CoreImage:          "ghcr.io/muxcore-media/muxcored:v0.5.4",
			Modules: []muxcorev1alpha1.ModuleSpec{{
				Name:  "api-rest",
				Image: "ghcr.io/muxcore-media/api-rest:v0.1.6",
				Port:  18080,
			}},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(platform).WithObjects(platform).Build()
	r := &controller.PlatformReconciler{Client: c, Scheme: scheme}

	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "demo", Namespace: "muxcore"},
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	var core appsv1.Deployment
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "muxcore", Name: "muxcored"}, &core); err != nil {
		t.Fatalf("get muxcored deployment: %v", err)
	}
	if core.Spec.Template.Spec.Containers[0].Image != "ghcr.io/muxcore-media/muxcored:v0.5.4" {
		t.Fatalf("core image = %q", core.Spec.Template.Spec.Containers[0].Image)
	}

	var api appsv1.Deployment
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "muxcore", Name: "api-rest"}, &api); err != nil {
		t.Fatalf("get api-rest deployment: %v", err)
	}

	var svc corev1.Service
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "muxcore", Name: "muxcored"}, &svc); err != nil {
		t.Fatalf("get muxcored service: %v", err)
	}

	var updated muxcorev1alpha1.MuxCorePlatform
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "muxcore", Name: "demo"}, &updated); err != nil {
		t.Fatalf("get platform: %v", err)
	}
	if updated.Status.DesiredModules != 2 {
		t.Fatalf("DesiredModules=%d want 2", updated.Status.DesiredModules)
	}
}

func TestPlatformReconciler_PrunesRemovedModules(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = muxcorev1alpha1.AddToScheme(scheme)

	platform := &muxcorev1alpha1.MuxCorePlatform{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "demo",
			Namespace:  "muxcore",
			Generation: 2,
		},
		Spec: muxcorev1alpha1.MuxCorePlatformSpec{
			InsecureDisableTLS: true,
			CoreImage:          "ghcr.io/muxcore-media/muxcored:v0.5.4",
			Modules:            []muxcorev1alpha1.ModuleSpec{},
		},
	}

	labels := map[string]string{
		"app.kubernetes.io/managed-by": "muxcore-operator",
		"muxcore.media/platform":         "demo",
		"app.kubernetes.io/part-of":      "muxcore",
		"app.kubernetes.io/component":    "api-rest",
	}
	staleDep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-rest",
			Namespace: "muxcore",
			Labels:    labels,
		},
	}
	staleSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-rest",
			Namespace: "muxcore",
			Labels:    labels,
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(platform).
		WithObjects(platform, staleDep, staleSvc).Build()
	r := &controller.PlatformReconciler{Client: c, Scheme: scheme}

	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "demo", Namespace: "muxcore"},
	})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	var gone appsv1.Deployment
	err = c.Get(context.Background(), types.NamespacedName{Namespace: "muxcore", Name: "api-rest"}, &gone)
	if err == nil {
		t.Fatal("expected stale deployment to be pruned")
	}
}
