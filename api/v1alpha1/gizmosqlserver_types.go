/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// GizmoSQLServerSpec defines the desired state of GizmoSQLServer.
// +kubebuilder:subresource:status
type GizmoSQLServerSpec struct {
	// Important: Run "make" to regenerate code after modifying this file

	// Repository and tag of the GizmoSQLServer image to use.
	Image GizmoSQLServerImage `json:"image,omitempty"`

	// Port defines the port to expose. Default is typically 31337 or similar for gizmosql.
	// +optional
	Port int32 `json:"port,omitempty" default:"31337"`

	// HealthCheckPort defines the port to expose for health check. Default is typically 31338 or similar for gizmosql.
	// +optional
	HealthCheckPort int32 `json:"healthCheckPort,omitempty" default:"31338"`

	// Authentication configuration. If no secretRef is provided, the GizmoSQLServer will use a default username of "gizmosql_username" and password of "gizmosql_password".
	Auth GizmoSQLServerAuth `json:"auth,omitempty"`

	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// NodeSelector defines the node selector for the GizmoSQLServer instance.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Affinity defines the affinity for the GizmoSQLServer instance.
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// Tolerations defines the tolerations for the GizmoSQLServer instance.
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
}

type GizmoSQLServerImage struct {
	Repository string            `json:"repository,omitempty"`
	Tag        string            `json:"tag,omitempty"`
	PullPolicy corev1.PullPolicy `json:"pullPolicy,omitempty" default:"Always"`
}

// GizmoSQLServerAuth defines the authentication configuration for the GizmoSQLServer.
type GizmoSQLServerAuth struct {
	SecretRef   corev1.SecretReference `json:"secretRef,omitempty"`
	UsernameKey string                 `json:"usernameKey,omitempty"`
	PasswordKey string                 `json:"passwordKey,omitempty"`
}

// GizmoSQLServerStatus defines the observed state of GizmoSQLServer.
type GizmoSQLServerStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Conditions store the status conditions of the GizmoSQLServer instances
	// +operator-sdk:csv:customresourcedefinitions:type=status
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// GizmoSQLServer is the Schema for the gizmosqlservers API.
type GizmoSQLServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GizmoSQLServerSpec   `json:"spec,omitempty"`
	Status GizmoSQLServerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GizmoSQLServerList contains a list of GizmoSQLServer.
type GizmoSQLServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GizmoSQLServer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GizmoSQLServer{}, &GizmoSQLServerList{})
}
