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

const duckdbFinalizer = "gizmodata.com/finalizer"

// Definitions to manage status conditions
const (
	// typeAvailableDuckDB represents the status of the StatefulSet reconciliation
	typeAvailableDuckDB = "Available"
	// typeDegradedDuckDB represents the status used when the custom resource is deleted and the finalizer operations are yet to occur.
	typeDegradedDuckDB = "Degraded"
)

// DuckDBReconciler reconciles a DuckDB object
type DuckDBReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=gizmodata.com,resources=duckdbs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gizmodata.com,resources=duckdbs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gizmodata.com,resources=duckdbs/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=core,resources=services,verbs=list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *DuckDBReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the DuckDB instance
	duckdb := &v1alpha1.DuckDB{}
	err := r.Get(ctx, req.NamespacedName, duckdb)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("duckdb resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get duckdb")
		return ctrl.Result{}, err
	}

	// Let's just set the status as Unknown when no status is available
	if len(duckdb.Status.Conditions) == 0 {
		meta.SetStatusCondition(&duckdb.Status.Conditions, metav1.Condition{Type: typeAvailableDuckDB, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})
		if err = r.Status().Update(ctx, duckdb); err != nil {
			log.Error(err, "Failed to update DuckDB status")
			return ctrl.Result{}, err
		}

		if err := r.Get(ctx, req.NamespacedName, duckdb); err != nil {
			log.Error(err, "Failed to re-fetch duckdb")
			return ctrl.Result{}, err
		}
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(duckdb, duckdbFinalizer) {
		log.Info("Adding Finalizer for DuckDB")
		if ok := controllerutil.AddFinalizer(duckdb, duckdbFinalizer); !ok {
			err = fmt.Errorf("finalizer for DuckDB was not added")
			log.Error(err, "Failed to add finalizer for DuckDB")
			return ctrl.Result{}, err
		}

		if err = r.Update(ctx, duckdb); err != nil {
			log.Error(err, "Failed to update custom resource to add finalizer")
			return ctrl.Result{}, err
		}
	}

	// Check if the DuckDB instance is marked to be deleted
	isDuckDBMarkedToBeDeleted := duckdb.GetDeletionTimestamp() != nil
	if isDuckDBMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(duckdb, duckdbFinalizer) {
			log.Info("Performing Finalizer Operations for DuckDB before delete CR")

			meta.SetStatusCondition(&duckdb.Status.Conditions, metav1.Condition{Type: typeDegradedDuckDB,
				Status: metav1.ConditionUnknown, Reason: "Finalizing",
				Message: fmt.Sprintf("Performing finalizer operations for the custom resource: %s ", duckdb.Name)})

			if err := r.Status().Update(ctx, duckdb); err != nil {
				log.Error(err, "Failed to update DuckDB status")
				return ctrl.Result{}, err
			}

			r.doFinalizerOperationsForDuckDB(duckdb)

			if err := r.Get(ctx, req.NamespacedName, duckdb); err != nil {
				log.Error(err, "Failed to re-fetch duckdb")
				return ctrl.Result{}, err
			}

			meta.SetStatusCondition(&duckdb.Status.Conditions, metav1.Condition{Type: typeDegradedDuckDB,
				Status: metav1.ConditionTrue, Reason: "Finalizing",
				Message: fmt.Sprintf("Finalizer operations for custom resource %s were successfully accomplished", duckdb.Name)})

			if err := r.Status().Update(ctx, duckdb); err != nil {
				log.Error(err, "Failed to update DuckDB status")
				return ctrl.Result{}, err
			}

			log.Info("Removing Finalizer for DuckDB")
			if ok := controllerutil.RemoveFinalizer(duckdb, duckdbFinalizer); !ok {
				err = fmt.Errorf("finalizer for DuckDB was not removed")
				log.Error(err, "Failed to remove finalizer for DuckDB")
				return ctrl.Result{}, err
			}

			if err := r.Update(ctx, duckdb); err != nil {
				log.Error(err, "Failed to remove finalizer for DuckDB")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Check if the StatefulSet already exists, if not create a new one
	foundStatefulSet := &appsv1.StatefulSet{}
	err = r.Get(ctx, types.NamespacedName{Name: duckdb.Name, Namespace: duckdb.Namespace}, foundStatefulSet)
	if err != nil && apierrors.IsNotFound(err) {
		// Define a new StatefulSet
		statefulSet, err := r.statefulSetForDuckDB(duckdb)
		if err != nil {
			log.Error(err, "Failed to define new StatefulSet resource for DuckDB")
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
	err = r.Get(ctx, types.NamespacedName{Name: duckdb.Name, Namespace: duckdb.Namespace}, foundService)
	if err != nil && apierrors.IsNotFound(err) {
		// Define a new Service
		service, err := r.serviceForDuckDB(duckdb)
		if err != nil {
			log.Error(err, "Failed to define new Service resource for DuckDB")
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

	// Update the DuckDB status with the statefulset names
	// List the statefulsets for this duckdb instance
	statefulSetList := &appsv1.StatefulSetList{}
	listOpts := []client.ListOption{
		client.InNamespace(duckdb.Namespace),
		client.MatchingLabels(labelsForDuckDB(duckdb.Name)),
	}
	if err = r.List(ctx, statefulSetList, listOpts...); err != nil {
		log.Error(err, "Failed to list StatefulSets", "DuckDB.Namespace", duckdb.Namespace, "DuckDB.Name", duckdb.Name)
		return ctrl.Result{}, err
	}

	// Update status.Conditions if needed
	meta.SetStatusCondition(&duckdb.Status.Conditions, metav1.Condition{Type: typeAvailableDuckDB,
		Status: metav1.ConditionTrue, Reason: "Reconciling",
		Message: fmt.Sprintf("StatefulSet for custom resource %s created successfully", duckdb.Name)})

	if err := r.Status().Update(ctx, duckdb); err != nil {
		log.Error(err, "Failed to update DuckDB status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// doFinalizerOperationsForDuckDB will perform the required operations before delete the CR.
func (r *DuckDBReconciler) doFinalizerOperationsForDuckDB(cr *v1alpha1.DuckDB) {
	// TODO(user): Add the cleanup steps that the operator
	// needs to do before the CR can be deleted.

	// The following implementation will raise an event
	r.Recorder.Event(cr, "Warning", "Deleting",
		fmt.Sprintf("Custom Resource %s is being deleted from the namespace %s",
			cr.Name,
			cr.Namespace))
}

func (r *DuckDBReconciler) serviceForDuckDB(duckdb *v1alpha1.DuckDB) (*corev1.Service, error) {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      duckdb.Name,
			Namespace: duckdb.Namespace,
			Labels:    labelsForDuckDB(duckdb.Name),
		},
		Spec: corev1.ServiceSpec{
			Selector: labelsForDuckDB(duckdb.Name),
			Ports: []corev1.ServicePort{{
				Port: duckdb.Spec.Port,
				Name: "duckdb",
			}},
		},
	}

	// Set the ownerRef for the Service
	if err := ctrl.SetControllerReference(duckdb, service, r.Scheme); err != nil {
		return nil, err
	}

	return service, nil
}

// statefulSetForDuckDB returns a DuckDB StatefulSet object
func (r *DuckDBReconciler) statefulSetForDuckDB(
	duckdb *v1alpha1.DuckDB) (*appsv1.StatefulSet, error) {
	// Use the image from Spec if provided, otherwise fallback or error
	// For now, we assume the user provides valid StatefulSetSpec or at least Image.
	// If Spec.Spec is empty, we construct a minimal one.
	image := duckdb.Spec.Image.Repository + ":" + duckdb.Spec.Image.Tag
	if duckdb.Spec.Image.Repository == "" {
		// Fallback to a default if not specified? Or error?
		// Example uses env var.
		var err error
		image, err = imageForDuckDB()
		if err != nil {
			// If no env var, and no spec, use a default placeholder or error
			// return nil, err
			// Let's use a placeholder for safety if no env var found
			image = "gizmodata/gizmosql:latest" // Placeholder
		}
	}

	if duckdb.Name == "" {
		// Generate a random kubernetes-friendly suffix for the name, e.g. 5-7 lowercase letters/numbers
		const nameSuffixLength = 6
		const letterBytes = "abcdefghijklmnopqrstuvwxyz0123456789"
		var suffix = make([]byte, nameSuffixLength)
		for i := range suffix {
			suffix[i] = letterBytes[time.Now().UnixNano()%int64(len(letterBytes))]
		}
		duckdb.Name = fmt.Sprintf("duckdb-%s", string(suffix))
	}

	auth := duckdb.Spec.Auth
	envVars := []corev1.EnvVar{}
	if auth.SecretRef.Name != "" {
		envVars = append(envVars, corev1.EnvVar{
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: auth.SecretRef.Name},
					Key:                  auth.PasswordKey,
				},
			},
		})
	} else {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "GIZMOSQL_PASSWORD",
			Value: "gizmosql_password",
		})
	}

	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      duckdb.Name,
			Namespace: duckdb.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labelsForDuckDB(duckdb.Name),
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image:           image,
						Name:            "duckdb",
						ImagePullPolicy: duckdb.Spec.Image.PullPolicy,
						Env:             envVars,
						Ports: []corev1.ContainerPort{{
							ContainerPort: duckdb.Spec.Port,
							Name:          "duckdb",
						}},
						Resources: duckdb.Spec.Resources,
					}},
					Affinity:     duckdb.Spec.Affinity,
					NodeSelector: duckdb.Spec.NodeSelector,
					Tolerations:  duckdb.Spec.Tolerations,
				},
				ObjectMeta: metav1.ObjectMeta{
					Labels: labelsForDuckDB(duckdb.Name),
				},
			},
			ServiceName: duckdb.Name,
		},
	}

	// Set the ownerRef for the StatefulSet
	if err := ctrl.SetControllerReference(duckdb, statefulSet, r.Scheme); err != nil {
		return nil, err
	}
	return statefulSet, nil
}

// labelsForDuckDB returns the labels for selecting the resources
func labelsForDuckDB(name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "duckdb",
		"app.kubernetes.io/instance":   name,
		"app.kubernetes.io/managed-by": "DuckDBController",
	}
}

// imageForDuckDB gets the Operand image which is managed by this controller
// from the DUCKDB_IMAGE environment variable defined in the config/manager/manager.yaml
func imageForDuckDB() (string, error) {
	var imageEnvVar = "DUCKDB_IMAGE"
	image, found := os.LookupEnv(imageEnvVar)
	if !found {
		return "", fmt.Errorf("unable to find %s environment variable with the image", imageEnvVar)
	}
	return image, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DuckDBReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.DuckDB{}).
		Owns(&appsv1.StatefulSet{}).
		Complete(r)
}
