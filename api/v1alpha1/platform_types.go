// Package v1alpha1 contains API Schema definitions for the muxcore v1alpha1 API group.
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// DefaultCoreImage is the default muxcored image (ghcr.io/muxcore-media).
	DefaultCoreImage = "ghcr.io/muxcore-media/muxcored:v0.5.4"
	// ReservedModuleName is injected automatically; must not appear in spec.modules.
	ReservedModuleName = "muxcored"
)

// VolumeSpec requests a PersistentVolumeClaim mounted into the module pod.
type VolumeSpec struct {
	// ClaimName is the PVC name (defaults to "<platform>-<module>-data").
	// +optional
	ClaimName string `json:"claimName,omitempty"`
	// Size is the storage request (e.g. "1Gi").
	// +optional
	Size string `json:"size,omitempty"`
	// MountPath is the container mount path (defaults to "/data").
	// +optional
	MountPath string `json:"mountPath,omitempty"`
	// StorageClassName selects a StorageClass (cluster default when empty).
	// +optional
	StorageClassName string `json:"storageClassName,omitempty"`
}

// ResourceSpec is a simplified resource requirements block for CRD JSON.
type ResourceSpec struct {
	// +optional
	Requests map[string]string `json:"requests,omitempty"`
	// +optional
	Limits map[string]string `json:"limits,omitempty"`
}

// ModuleSpec describes one MuxCore sidecar Deployment owned by a Platform.
type ModuleSpec struct { //nolint:govet // fieldalignment: JSON field order matches kubebuilder CRD schema
	// Name is the module id (e.g. api-rest). Deployment name is "<platform>-<name>".
	Name string `json:"name"`
	// Image is the container image (e.g. ghcr.io/muxcore-media/…).
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
	// Port is the legacy primary Service port when httpServicePort is unset.
	// +optional
	Port int32 `json:"port,omitempty"`
	// HttpPort is the container HTTP listen port.
	// +optional
	HttpPort int32 `json:"httpPort,omitempty"`
	// HttpServicePort is the Service HTTP port (defaults to Port or HttpPort).
	// +optional
	HttpServicePort int32 `json:"httpServicePort,omitempty"`
	// GrpcPort is the container and Service gRPC port.
	// +optional
	GrpcPort int32 `json:"grpcPort,omitempty"`
	// HealthPath enables HTTP readiness/liveness probes on httpPort.
	// +optional
	HealthPath string `json:"healthPath,omitempty"`
	// Volume requests a PVC and mount for stateful modules.
	// +optional
	Volume *VolumeSpec `json:"volume,omitempty"`
	// Resources overrides platform defaultResources for this module.
	// +optional
	Resources *ResourceSpec `json:"resources,omitempty"`
}

// MuxCorePlatformSpec defines the desired state of MuxCorePlatform.
type MuxCorePlatformSpec struct { //nolint:govet // fieldalignment: JSON field order matches kubebuilder CRD schema
	// InsecureDisableTLS sets MUXCORE_INSECURE_DISABLE_TLS on managed pods.
	// When false, meshTLSSecret must supply cert material.
	// +optional
	InsecureDisableTLS bool `json:"insecureDisableTLS,omitempty"`
	// CoreImage is the muxcored container image.
	// +kubebuilder:default="ghcr.io/muxcore-media/muxcored:v0.5.4"
	CoreImage string `json:"coreImage,omitempty"`
	// MeshAddr overrides the dial address passed as MUXCORE_GRPC_ADDR on sidecars.
	// Defaults to "<platform>-muxcored:9090".
	// +optional
	MeshAddr string `json:"meshAddr,omitempty"`
	// MeshTLSSecret names a Secret with tls.crt, tls.key, and ca.crt for mesh mTLS.
	// Required when insecureDisableTLS is false.
	// +optional
	MeshTLSSecret string `json:"meshTLSSecret,omitempty"`
	// ImagePullPolicy for all managed pods (defaults to IfNotPresent).
	// +optional
	ImagePullPolicy string `json:"imagePullPolicy,omitempty"`
	// ImagePullSecrets for private registries.
	// +optional
	ImagePullSecrets []string `json:"imagePullSecrets,omitempty"`
	// DefaultResources applied to every module unless overridden.
	// +optional
	DefaultResources *ResourceSpec `json:"defaultResources,omitempty"`
	// Modules lists sidecar modules to reconcile as Deployments.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self.all(m, m.name.matches('^[a-z0-9]([-a-z0-9]*[a-z0-9])?$'))",message="module names must be DNS-1123 labels"
	// +kubebuilder:validation:XValidation:rule="self.all(m, m.name != 'muxcored')",message="muxcored is reserved for the core Deployment"
	// +kubebuilder:validation:XValidation:rule="self.size() == self.map(m, m.name).size()",message="module names must be unique"
	Modules []ModuleSpec `json:"modules,omitempty"`
}

// MuxCorePlatformStatus defines the observed state of MuxCorePlatform.
type MuxCorePlatformStatus struct { //nolint:govet // fieldalignment: JSON field order matches kubebuilder CRD schema
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
