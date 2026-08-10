// Package v1alpha1 contains API Schema definitions for the muxcore v1alpha1 API group.
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ModuleSpec describes one MuxCore sidecar Deployment owned by a Platform.
type ModuleSpec struct {
	// Name is the module id / Deployment name (e.g. api-rest).
	Name string `json:"name"`
	// Image is the container image (ghcr.io/muxcore-media/...).
	Image string `json:"image"`
	// Args are appended to the container command.
	// +optional
	Args []string `json:"args,omitempty"`
	// Env is a simple key/value map (use Secrets via EnvFromSecret for credentials).
	// +optional
	Env map[string]string `json:"env,omitempty"`
	// EnvFromSecret mounts all keys from a Secret as environment variables.
	// +optional
	EnvFromSecret string `json:"envFromSecret,omitempty"`
	// Port is the primary container port (mesh gRPC or HTTP).
	// +optional
	Port int32 `json:"port,omitempty"`
}

// MuxCorePlatformSpec defines the desired state of MuxCorePlatform.
type MuxCorePlatformSpec struct {
	// InsecureDisableTLS sets MUXCORE_INSECURE_DISABLE_TLS on managed pods.
	// +optional
	InsecureDisableTLS bool `json:"insecureDisableTLS,omitempty"`
	// CoreImage is the muxcored container image.
	// +kubebuilder:default="ghcr.io/muxcore-media/muxcored:v0.5.4"
	CoreImage string `json:"coreImage,omitempty"`
	// MeshAddr is the gRPC mesh listen address passed to modules.
	// +kubebuilder:default="muxcored:9090"
	MeshAddr string `json:"meshAddr,omitempty"`
	// Modules lists sidecar modules to reconcile as Deployments.
	// +optional
	Modules []ModuleSpec `json:"modules,omitempty"`
}

// MuxCorePlatformStatus defines the observed state of MuxCorePlatform.
type MuxCorePlatformStatus struct {
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// ReadyModules is the count of module Deployments with AvailableReplicas >= 1.
	ReadyModules int32 `json:"readyModules,omitempty"`
	// DesiredModules is len(spec.modules) + 1 (core).
	DesiredModules int32 `json:"desiredModules,omitempty"`
	// Conditions mirror common Ready status.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=mcp
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Modules",type=string,JSONPath=`.status.readyModules`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// MuxCorePlatform is the Schema for the muxcoreplatforms API.
// It describes a minimal MuxCore control plane (muxcored + sidecars) in one namespace.
type MuxCorePlatform struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MuxCorePlatformSpec   `json:"spec,omitempty"`
	Status MuxCorePlatformStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MuxCorePlatformList contains a list of MuxCorePlatform.
type MuxCorePlatformList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MuxCorePlatform `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MuxCorePlatform{}, &MuxCorePlatformList{})
}
