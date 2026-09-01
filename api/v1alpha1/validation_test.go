package v1alpha1_test

import (
	"strings"
	"testing"

	muxcorev1alpha1 "github.com/Muxcore-Media/muxcore-operator/api/v1alpha1"
)

func TestValidateModules(t *testing.T) {
	if err := muxcorev1alpha1.ValidateModules(nil); err != nil {
		t.Fatalf("nil modules: %v", err)
	}
	if err := muxcorev1alpha1.ValidateModules([]muxcorev1alpha1.ModuleSpec{
		{Name: "api-rest", Image: "x"},
	}); err != nil {
		t.Fatalf("valid: %v", err)
	}
	err := muxcorev1alpha1.ValidateModules([]muxcorev1alpha1.ModuleSpec{
		{Name: "muxcored", Image: "x"},
	})
	if err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("reserved: %v", err)
	}
	err = muxcorev1alpha1.ValidateModules([]muxcorev1alpha1.ModuleSpec{
		{Name: "Bad_Name", Image: "x"},
	})
	if err == nil || !strings.Contains(err.Error(), "DNS-1123") {
		t.Fatalf("dns: %v", err)
	}
}
