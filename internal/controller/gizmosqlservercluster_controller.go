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

// GizmoSQLServerClusterReconciler reconciles a GizmoSQLServerCluster object
type GizmoSQLServerClusterReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// Definitions to manage status conditions
const (
	// typeAvailableGizmoSQLServerCluster represents the status of the StatefulSet reconciliation
	typeAvailableGizmoSQLServerCluster = "Available"
	// typeDegradedGizmoSQLServerCluster represents the status used when the custom resource is deleted and the finalizer operations are yet to occur.
	typeDegradedGizmoSQLServerCluster = "Degraded"
)

// +kubebuilder:rbac:groups=gizmodata.com,resources=gizmosqlserverclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gizmodata.com,resources=gizmosqlserverclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gizmodata.com,resources=gizmosqlserverclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=core,resources=services,verbs=list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the GizmoSQLServerCluster object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.4/pkg/reconcile
func (r *GizmoSQLServerClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the GizmoSQLServerCluster instance
	gizmoSQLServerCluster := &v1alpha1.GizmoSQLServerCluster{}
	err := r.Get(ctx, req.NamespacedName, gizmoSQLServerCluster)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("gizmosqlservercluster resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get gizmosqlservercluster")
		return ctrl.Result{}, err
	}

	// Let's just set the status as Unknown when no status is available
	if len(gizmoSQLServerCluster.Status.Conditions) == 0 {
		meta.SetStatusCondition(&gizmoSQLServerCluster.Status.Conditions, metav1.Condition{Type: typeAvailableGizmoSQLServerCluster, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})
		if err = r.Status().Update(ctx, gizmoSQLServerCluster); err != nil {
			log.Error(err, "Failed to update GizmoSQLServerCluster status")
			return ctrl.Result{}, err
		}

		if err := r.Get(ctx, req.NamespacedName, gizmoSQLServerCluster); err != nil {
			log.Error(err, "Failed to re-fetch gizmosqlservercluster")
			return ctrl.Result{}, err
		}
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(gizmoSQLServerCluster, GizmoSQLServerClusterFinalizer) {
		log.Info("Adding Finalizer for GizmoSQLServerCluster")
		if ok := controllerutil.AddFinalizer(gizmoSQLServerCluster, GizmoSQLServerClusterFinalizer); !ok {
			err = fmt.Errorf("finalizer for GizmoSQLServerCluster was not added")
			log.Error(err, "Failed to add finalizer for GizmoSQLServerCluster")
			return ctrl.Result{}, err
		}

		if err = r.Update(ctx, gizmoSQLServerCluster); err != nil {
			log.Error(err, "Failed to update custom resource to add finalizer")
			return ctrl.Result{}, err
		}
	}

	isGizmoSQLServerClusterMarkedToBeDeleted := gizmoSQLServerCluster.GetDeletionTimestamp() != nil
	if isGizmoSQLServerClusterMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(gizmoSQLServerCluster, GizmoSQLServerClusterFinalizer) {
			log.Info("Performing Finalizer Operations for GizmoSQLServerCluster before delete CR")

			meta.SetStatusCondition(&gizmoSQLServerCluster.Status.Conditions, metav1.Condition{Type: typeDegradedGizmoSQLServerCluster,
				Status: metav1.ConditionUnknown, Reason: "Finalizing",
				Message: fmt.Sprintf("Performing finalizer operations for the custom resource: %s ", gizmoSQLServerCluster.Name)})

			if err := r.Status().Update(ctx, gizmoSQLServerCluster); err != nil {
				log.Error(err, "Failed to update GizmoSQLServerCluster status")
				return ctrl.Result{}, err
			}

			r.doFinalizerOperationsForGizmoSQLServerCluster(gizmoSQLServerCluster)

			if err := r.Get(ctx, req.NamespacedName, gizmoSQLServerCluster); err != nil {
				log.Error(err, "Failed to re-fetch gizmosqlservercluster")
				return ctrl.Result{}, err
			}

			meta.SetStatusCondition(&gizmoSQLServerCluster.Status.Conditions, metav1.Condition{Type: typeDegradedGizmoSQLServerCluster,
				Status: metav1.ConditionTrue, Reason: "Finalizing",
				Message: fmt.Sprintf("Finalizer operations for custom resource %s were successfully accomplished", gizmoSQLServerCluster.Name)})

			if err := r.Status().Update(ctx, gizmoSQLServerCluster); err != nil {
				log.Error(err, "Failed to update GizmoSQLServerCluster status")
				return ctrl.Result{}, err
			}

			log.Info("Removing Finalizer for GizmoSQLServerCluster")
			if ok := controllerutil.RemoveFinalizer(gizmoSQLServerCluster, GizmoSQLServerClusterFinalizer); !ok {
				err = fmt.Errorf("finalizer for GizmoSQLServerCluster was not removed")
				log.Error(err, "Failed to remove finalizer for GizmoSQLServerCluster")
				return ctrl.Result{}, err
			}

			if err := r.Update(ctx, gizmoSQLServerCluster); err != nil {
				log.Error(err, "Failed to remove finalizer for GizmoSQLServerCluster")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Check if the Deployment already exists, if not create a new one
	foundDeployment := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: gizmoSQLServerCluster.Name, Namespace: gizmoSQLServerCluster.Namespace}, foundDeployment)
	if err != nil && apierrors.IsNotFound(err) {
		// Define a new Deployment
		deployment, err := r.deploymentForGizmoSQLServerCluster(gizmoSQLServerCluster)
		if err != nil {
			log.Error(err, "Failed to define new Deployment resource for GizmoSQLServerCluster")
			return ctrl.Result{}, err
		}

		log.Info("Creating a new Deployment", "Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)
		if err = r.Create(ctx, deployment); err != nil {
			log.Error(err, "Failed to create new Deployment", "Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)
			return ctrl.Result{}, err
		}

		// Deployment created successfully
		// We will requeue the reconciliation so that we can ensure the state
		// and move forward for the next operations
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Deployment")
		return ctrl.Result{}, err
	}

	// Check if the Service already exists, if not create a new one
	foundService := &corev1.Service{}
	err = r.Get(ctx, types.NamespacedName{Name: gizmoSQLServerCluster.Name, Namespace: gizmoSQLServerCluster.Namespace}, foundService)
	if err != nil && apierrors.IsNotFound(err) {
		// Define a new Service
		service := ServiceForGizmoSQLServerSpect(gizmoSQLServerCluster.Name, gizmoSQLServerCluster.Namespace, &gizmoSQLServerCluster.Spec.ServerSpec)
		if err := ctrl.SetControllerReference(gizmoSQLServerCluster, service, r.Scheme); err != nil {
			log.Error(err, "Failed to set controller reference for Service")
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

	// Update the GizmoSQLServerCluster status with the deployment names
	// List the deployments for this gizmosqlservercluster instance
	deploymentList := &appsv1.DeploymentList{}
	listOpts := []client.ListOption{
		client.InNamespace(gizmoSQLServerCluster.Namespace),
		client.MatchingLabels(DefaultLabelsForGizmoSQLServer(gizmoSQLServerCluster.Name)),
	}
	if err = r.List(ctx, deploymentList, listOpts...); err != nil {
		log.Error(err, "Failed to list Deployments", "GizmoSQLServerCluster.Namespace", gizmoSQLServerCluster.Namespace, "GizmoSQLServerCluster.Name", gizmoSQLServerCluster.Name)
		return ctrl.Result{}, err
	}

	// Update status.Conditions if needed
	meta.SetStatusCondition(&gizmoSQLServerCluster.Status.Conditions, metav1.Condition{Type: typeAvailableGizmoSQLServerCluster,
		Status: metav1.ConditionTrue, Reason: "Reconciling",
		Message: fmt.Sprintf("Deployment for custom resource %s created successfully", gizmoSQLServerCluster.Name)})

	if err := r.Status().Update(ctx, gizmoSQLServerCluster); err != nil {
		log.Error(err, "Failed to update GizmoSQLServerCluster status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// doFinalizerOperationsForGizmoSQLServer will perform the required operations before delete the CR.
func (r *GizmoSQLServerClusterReconciler) doFinalizerOperationsForGizmoSQLServerCluster(cr *v1alpha1.GizmoSQLServerCluster) {
	// TODO(user): Add the cleanup steps that the operator
	// needs to do before the CR can be deleted.

	// The following implementation will raise an event
	r.Recorder.Event(cr, "Warning", "Deleting",
		fmt.Sprintf("Custom Resource %s is being deleted from the namespace %s",
			cr.Name,
			cr.Namespace))
}

func (r *GizmoSQLServerClusterReconciler) deploymentForGizmoSQLServerCluster(gizmoSQLServerCluster *v1alpha1.GizmoSQLServerCluster) (*appsv1.Deployment, error) {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gizmoSQLServerCluster.Name,
			Namespace: gizmoSQLServerCluster.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: DefaultLabelsForGizmoSQLServer(gizmoSQLServerCluster.Name),
			},
			Replicas: gizmoSQLServerCluster.Spec.Replicas,
			Template: corev1.PodTemplateSpec{
				Spec: *PodSpecFromGizmoSQLServerSpec(&gizmoSQLServerCluster.Spec.ServerSpec),
				ObjectMeta: metav1.ObjectMeta{
					Labels: DefaultLabelsForGizmoSQLServer(gizmoSQLServerCluster.Name),
				},
			},
		},
	}

	// Set the ownerRef for the Service
	if err := ctrl.SetControllerReference(gizmoSQLServerCluster, deployment, r.Scheme); err != nil {
		return nil, err
	}

	return deployment, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GizmoSQLServerClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.GizmoSQLServerCluster{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}
