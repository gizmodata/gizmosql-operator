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

package controller

import (
	"context"
	"fmt"
	"os"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	v1alpha1 "github.com/gizmodata/gizmosql-operator/api/v1alpha1"
)

const gizmoSQLServerFinalizer = "gizmodata.com/finalizer"

const (
	gizmoSQLUsernameEnvVar = "GIZMOSQL_USERNAME"
	gizmoSQLPasswordEnvVar = "GIZMOSQL_PASSWORD"
)

// Definitions to manage status conditions
const (
	// typeAvailableGizmoSQLServer represents the status of the StatefulSet reconciliation
	typeAvailableGizmoSQLServer = "Available"
	// typeDegradedGizmoSQLServer represents the status used when the custom resource is deleted and the finalizer operations are yet to occur.
	typeDegradedGizmoSQLServer = "Degraded"
)

// GizmoSQLServerReconciler reconciles a GizmoSQLServer object
type GizmoSQLServerReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=gizmodata.com,resources=gizmosqlservers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gizmodata.com,resources=gizmosqlservers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gizmodata.com,resources=gizmosqlservers/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=core,resources=services,verbs=list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *GizmoSQLServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the GizmoSQLServer instance
	gizmoSQLServer := &v1alpha1.GizmoSQLServer{}
	err := r.Get(ctx, req.NamespacedName, gizmoSQLServer)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("gizmosqlserver resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get gizmosqlserver")
		return ctrl.Result{}, err
	}

	// Let's just set the status as Unknown when no status is available
	if len(gizmoSQLServer.Status.Conditions) == 0 {
		meta.SetStatusCondition(&gizmoSQLServer.Status.Conditions, metav1.Condition{Type: typeAvailableGizmoSQLServer, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})
		if err = r.Status().Update(ctx, gizmoSQLServer); err != nil {
			log.Error(err, "Failed to update GizmoSQLServer status")
			return ctrl.Result{}, err
		}

		if err := r.Get(ctx, req.NamespacedName, gizmoSQLServer); err != nil {
			log.Error(err, "Failed to re-fetch gizmosqlserver")
			return ctrl.Result{}, err
		}
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(gizmoSQLServer, gizmoSQLServerFinalizer) {
		log.Info("Adding Finalizer for GizmoSQLServer")
		if ok := controllerutil.AddFinalizer(gizmoSQLServer, gizmoSQLServerFinalizer); !ok {
			err = fmt.Errorf("finalizer for GizmoSQLServer was not added")
			log.Error(err, "Failed to add finalizer for GizmoSQLServer")
			return ctrl.Result{}, err
		}

		if err = r.Update(ctx, gizmoSQLServer); err != nil {
			log.Error(err, "Failed to update custom resource to add finalizer")
			return ctrl.Result{}, err
		}
	}

	// Check if the GizmoSQLServer instance is marked to be deleted
	isGizmoSQLServerMarkedToBeDeleted := gizmoSQLServer.GetDeletionTimestamp() != nil
	if isGizmoSQLServerMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(gizmoSQLServer, gizmoSQLServerFinalizer) {
			log.Info("Performing Finalizer Operations for GizmoSQLServer before delete CR")

			meta.SetStatusCondition(&gizmoSQLServer.Status.Conditions, metav1.Condition{Type: typeDegradedGizmoSQLServer,
				Status: metav1.ConditionUnknown, Reason: "Finalizing",
				Message: fmt.Sprintf("Performing finalizer operations for the custom resource: %s ", gizmoSQLServer.Name)})

			if err := r.Status().Update(ctx, gizmoSQLServer); err != nil {
				log.Error(err, "Failed to update GizmoSQLServer status")
				return ctrl.Result{}, err
			}

			r.doFinalizerOperationsForGizmoSQLServer(gizmoSQLServer)

			if err := r.Get(ctx, req.NamespacedName, gizmoSQLServer); err != nil {
				log.Error(err, "Failed to re-fetch gizmosqlserver")
				return ctrl.Result{}, err
			}

			meta.SetStatusCondition(&gizmoSQLServer.Status.Conditions, metav1.Condition{Type: typeDegradedGizmoSQLServer,
				Status: metav1.ConditionTrue, Reason: "Finalizing",
				Message: fmt.Sprintf("Finalizer operations for custom resource %s were successfully accomplished", gizmoSQLServer.Name)})

			if err := r.Status().Update(ctx, gizmoSQLServer); err != nil {
				log.Error(err, "Failed to update GizmoSQLServer status")
				return ctrl.Result{}, err
			}

			log.Info("Removing Finalizer for GizmoSQLServer")
			if ok := controllerutil.RemoveFinalizer(gizmoSQLServer, gizmoSQLServerFinalizer); !ok {
				err = fmt.Errorf("finalizer for GizmoSQLServer was not removed")
				log.Error(err, "Failed to remove finalizer for GizmoSQLServer")
				return ctrl.Result{}, err
			}

			if err := r.Update(ctx, gizmoSQLServer); err != nil {
				log.Error(err, "Failed to remove finalizer for GizmoSQLServer")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Check if the StatefulSet already exists, if not create a new one
	foundStatefulSet := &appsv1.StatefulSet{}
	err = r.Get(ctx, types.NamespacedName{Name: gizmoSQLServer.Name, Namespace: gizmoSQLServer.Namespace}, foundStatefulSet)
	if err != nil && apierrors.IsNotFound(err) {
		// Define a new StatefulSet
		statefulSet, err := r.statefulSetForGizmoSQLServer(gizmoSQLServer)
		if err != nil {
			log.Error(err, "Failed to define new StatefulSet resource for GizmoSQLServer")
			return ctrl.Result{}, err
		}

		log.Info("Creating a new StatefulSet", "StatefulSet.Namespace", statefulSet.Namespace, "StatefulSet.Name", statefulSet.Name)
		if err = r.Create(ctx, statefulSet); err != nil {
			log.Error(err, "Failed to create new StatefulSet", "StatefulSet.Namespace", statefulSet.Namespace, "StatefulSet.Name", statefulSet.Name)
			return ctrl.Result{}, err
		}

		// StatefulSet created successfully
		// We will requeue the reconciliation so that we can ensure the state
		// and move forward for the next operations
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if err != nil {
		log.Error(err, "Failed to get StatefulSet")
		return ctrl.Result{}, err
	}

	// Check if the Service already exists, if not create a new one
	foundService := &corev1.Service{}
	err = r.Get(ctx, types.NamespacedName{Name: gizmoSQLServer.Name, Namespace: gizmoSQLServer.Namespace}, foundService)
	if err != nil && apierrors.IsNotFound(err) {
		// Define a new Service
		service, err := r.serviceForGizmoSQLServer(gizmoSQLServer)
		if err != nil {
			log.Error(err, "Failed to define new Service resource for GizmoSQLServer")
			return ctrl.Result{}, err
		}

		log.Info("Creating a new Service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
		if err = r.Create(ctx, service); err != nil {
			log.Error(err, "Failed to create new Service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
			return ctrl.Result{}, err
		}

		// Service created successfully
		// We will requeue the reconciliation so that we can ensure the state
		// and move forward for the next operations
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Service")
		return ctrl.Result{}, err
	}

	// Update the GizmoSQLServer status with the statefulset names
	// List the statefulsets for this gizmosqlserver instance
	statefulSetList := &appsv1.StatefulSetList{}
	listOpts := []client.ListOption{
		client.InNamespace(gizmoSQLServer.Namespace),
		client.MatchingLabels(labelsForGizmoSQLServer(gizmoSQLServer.Name)),
	}
	if err = r.List(ctx, statefulSetList, listOpts...); err != nil {
		log.Error(err, "Failed to list StatefulSets", "GizmoSQLServer.Namespace", gizmoSQLServer.Namespace, "GizmoSQLServer.Name", gizmoSQLServer.Name)
		return ctrl.Result{}, err
	}

	// Update status.Conditions if needed
	meta.SetStatusCondition(&gizmoSQLServer.Status.Conditions, metav1.Condition{Type: typeAvailableGizmoSQLServer,
		Status: metav1.ConditionTrue, Reason: "Reconciling",
		Message: fmt.Sprintf("StatefulSet for custom resource %s created successfully", gizmoSQLServer.Name)})

	if err := r.Status().Update(ctx, gizmoSQLServer); err != nil {
		log.Error(err, "Failed to update GizmoSQLServer status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// doFinalizerOperationsForGizmoSQLServer will perform the required operations before delete the CR.
func (r *GizmoSQLServerReconciler) doFinalizerOperationsForGizmoSQLServer(cr *v1alpha1.GizmoSQLServer) {
	// TODO(user): Add the cleanup steps that the operator
	// needs to do before the CR can be deleted.

	// The following implementation will raise an event
	r.Recorder.Event(cr, "Warning", "Deleting",
		fmt.Sprintf("Custom Resource %s is being deleted from the namespace %s",
			cr.Name,
			cr.Namespace))
}

func (r *GizmoSQLServerReconciler) serviceForGizmoSQLServer(gizmoSQLServer *v1alpha1.GizmoSQLServer) (*corev1.Service, error) {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gizmoSQLServer.Name,
			Namespace: gizmoSQLServer.Namespace,
			Labels:    labelsForGizmoSQLServer(gizmoSQLServer.Name),
		},
		Spec: corev1.ServiceSpec{
			Selector: labelsForGizmoSQLServer(gizmoSQLServer.Name),
			Ports: []corev1.ServicePort{{
				Port: gizmoSQLServer.Spec.Port,
				Name: "gizmosqlserver",
			}},
		},
	}

	// Set the ownerRef for the Service
	if err := ctrl.SetControllerReference(gizmoSQLServer, service, r.Scheme); err != nil {
		return nil, err
	}

	return service, nil
}

// statefulSetForGizmoSQLServer returns a GizmoSQLServer StatefulSet object
func (r *GizmoSQLServerReconciler) statefulSetForGizmoSQLServer(
	gizmoSQLServer *v1alpha1.GizmoSQLServer) (*appsv1.StatefulSet, error) {
	// Use the image from Spec if provided, otherwise fallback or error
	// For now, we assume the user provides valid StatefulSetSpec or at least Image.
	// If Spec.Spec is empty, we construct a minimal one.
	image := gizmoSQLServer.Spec.Image.Repository + ":" + gizmoSQLServer.Spec.Image.Tag
	if gizmoSQLServer.Spec.Image.Repository == "" {
		// Fallback to a default if not specified? Or error?
		// Example uses env var.
		var err error
		image, err = imageForGizmoSQLServer()
		if err != nil {
			// If no env var, and no spec, use a default placeholder or error
			// return nil, err
			// Let's use a placeholder for safety if no env var found
			image = "gizmodata/gizmosql:latest" // Placeholder
		}
	}

	auth := gizmoSQLServer.Spec.Auth
	envVars := []corev1.EnvVar{}
	if auth.SecretRef.Name != "" {
		envVars = append(envVars,
			corev1.EnvVar{
				Name: gizmoSQLUsernameEnvVar,
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: auth.SecretRef.Name},
						Key:                  auth.UsernameKey,
					},
				},
			},
			corev1.EnvVar{
				Name: gizmoSQLPasswordEnvVar,
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: auth.SecretRef.Name},
						Key:                  auth.PasswordKey,
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

	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gizmoSQLServer.Name,
			Namespace: gizmoSQLServer.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labelsForGizmoSQLServer(gizmoSQLServer.Name),
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image:           image,
						Name:            "gizmosqlserver",
						ImagePullPolicy: gizmoSQLServer.Spec.Image.PullPolicy,
						Env:             envVars,
						Ports: []corev1.ContainerPort{{
							ContainerPort: gizmoSQLServer.Spec.Port,
							Name:          "gizmosqlserver",
						}},
						Resources: gizmoSQLServer.Spec.Resources,
					}},
					Affinity:     gizmoSQLServer.Spec.Affinity,
					NodeSelector: gizmoSQLServer.Spec.NodeSelector,
					Tolerations:  gizmoSQLServer.Spec.Tolerations,
				},
				ObjectMeta: metav1.ObjectMeta{
					Labels: labelsForGizmoSQLServer(gizmoSQLServer.Name),
				},
			},
			ServiceName: gizmoSQLServer.Name,
		},
	}

	// Set the ownerRef for the StatefulSet
	if err := ctrl.SetControllerReference(gizmoSQLServer, statefulSet, r.Scheme); err != nil {
		return nil, err
	}
	return statefulSet, nil
}

// labelsForGizmoSQLServer returns the labels for selecting the resources
func labelsForGizmoSQLServer(name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "gizmosqlserver",
		"app.kubernetes.io/instance":   name,
		"app.kubernetes.io/managed-by": "GizmoSQLServerController",
	}
}

// imageForGizmoSQLServer gets the Operand image which is managed by this controller
// from the GIZMOSQLSERVER_IMAGE environment variable defined in the config/manager/manager.yaml
func imageForGizmoSQLServer() (string, error) {
	var imageEnvVar = "GIZMOSQLSERVER_IMAGE"
	image, found := os.LookupEnv(imageEnvVar)
	if !found {
		return "", fmt.Errorf("unable to find %s environment variable with the image", imageEnvVar)
	}
	return image, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GizmoSQLServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.GizmoSQLServer{}).
		Owns(&appsv1.StatefulSet{}).
		Complete(r)
}
