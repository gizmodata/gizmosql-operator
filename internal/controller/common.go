package controller

import (
	v1alpha1 "github.com/gizmodata/gizmosql-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// labelsForGizmoSQLServer returns the labels for selecting the resources
func DefaultLabelsForGizmoSQLServer(name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       DefaultGizmoSQLServerName,
		"app.kubernetes.io/instance":   name,
		"app.kubernetes.io/managed-by": DefaultGizmoSQLServerControllerName,
	}
}

func EnvVarsFromGizmoSQLServerSpec(spec *v1alpha1.GizmoSQLServerSpec) []corev1.EnvVar {
	envVars := []corev1.EnvVar{}
	if spec.Auth.SecretRef.Name != "" {
		envVars = append(envVars,
			corev1.EnvVar{
				Name: gizmoSQLUsernameEnvVar,
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: spec.Auth.SecretRef.Name},
						Key:                  spec.Auth.UsernameKey,
					},
				},
			},
			corev1.EnvVar{
				Name: gizmoSQLPasswordEnvVar,
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: spec.Auth.SecretRef.Name},
						Key:                  spec.Auth.PasswordKey,
					},
				},
			},
		)
	} else {
		envVars = append(envVars,
			corev1.EnvVar{
				Name:  gizmoSQLUsernameEnvVar,
				Value: "gizmosql_username",
			},
			corev1.EnvVar{
				Name:  gizmoSQLPasswordEnvVar,
				Value: "gizmosql_password",
			},
		)
	}
	return envVars
}

func PodSpecFromGizmoSQLServerSpec(spec *v1alpha1.GizmoSQLServerSpec) *corev1.PodSpec {
	repository := spec.Image.Repository
	if repository == "" {
		repository = DefaultGizmoSQLServerImageRepository
	}
	tag := spec.Image.Tag
	if tag == "" {
		tag = DefaultGizmoSQLServerImageTag
	}
	image := repository + ":" + tag
	port := spec.Port
	if port == 0 {
		port = DefaultGizmoSQLServerPort
	}
	healthCheckPort := spec.HealthCheckPort
	if healthCheckPort == 0 {
		healthCheckPort = DefaultGizmoSQLServerHealthCheckPort
	}

	envVars := EnvVarsFromGizmoSQLServerSpec(spec)

	return &corev1.PodSpec{
		Containers: []corev1.Container{{
			Image:           image,
			Name:            DefaultGizmoSQLServerContainerName,
			ImagePullPolicy: spec.Image.PullPolicy,
			Env:             envVars,
			Ports: []corev1.ContainerPort{
				{
					ContainerPort: port,
					Name:          DefaultGizmoSQLServicePortName,
				},
				{
					ContainerPort: healthCheckPort,
					Name:          DefaultGizmoSQLServerHealthCheckPortName,
				},
			},
			Resources: spec.Resources,
			LivenessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					GRPC: &corev1.GRPCAction{
						Port: healthCheckPort,
					},
				},
				InitialDelaySeconds: DefaultGizmoSQLServerLivenessProbeInitialDelaySeconds,
				PeriodSeconds:       DefaultGizmoSQLServerLivenessProbePeriodSeconds,
			},
			ReadinessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					GRPC: &corev1.GRPCAction{
						Port: healthCheckPort,
					},
				},
				InitialDelaySeconds: DefaultGizmoSQLServerReadinessProbeInitialDelaySeconds,
				PeriodSeconds:       DefaultGizmoSQLServerReadinessProbePeriodSeconds,
			},
		}},
		Affinity:     spec.Affinity,
		NodeSelector: spec.NodeSelector,
		Tolerations:  spec.Tolerations,
	}
}

func ServiceForGizmoSQLServerSpect(name string, namespace string, spec *v1alpha1.GizmoSQLServerSpec) *corev1.Service {
	port := spec.Port
	if port == 0 {
		port = DefaultGizmoSQLServerPort
	}
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    DefaultLabelsForGizmoSQLServer(name),
		},
		Spec: corev1.ServiceSpec{
			Selector: DefaultLabelsForGizmoSQLServer(name),
			Ports: []corev1.ServicePort{{
				Port: port,
				Name: DefaultGizmoSQLServicePortName,
			}},
		},
	}

	return service
}
