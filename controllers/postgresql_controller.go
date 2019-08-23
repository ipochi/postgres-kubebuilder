/*

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

package controllers

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"context"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	databasev1alpha1 "github.com/ipochi/kubebuilder-postgresql/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/resource"
	//"k8s.io/apimachinery/pkg/util/intstr"
)

// PostgreSQLReconciler reconciles a PostgreSQL object
type PostgreSQLReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=database.ourpostgres.com,resources=postgresqls,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=database.ourpostgres.com,resources=postgresqls/status,verbs=get;update;patch

func (r *PostgreSQLReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	_ = r.Log.WithValues("postgresql", req.NamespacedName)

	// Get Postgres instance
	postgresql := &databasev1alpha1.PostgreSQL{}
	if err := r.Get(ctx, req.NamespacedName, postgresql); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "postgres-config",
			// Labels: map[string]string{
			// 	"app": "postgres",
			// },
		},
		Data: map[string]string{
			"POSTGRES_DB":       "postgresdb",
			"POSTGRES_USER":     "postgresadmin",
			"POSTGRES_PASSWORD": "admin",
		},
	}

	_, err := ctrl.CreateOrUpdate(ctx, r.Client, configMap, func() error {

		copyLabels := postgresql.GetLabels()
		if copyLabels == nil {
			copyLabels = map[string]string{}
		}
		labels := map[string]string{}
		for k, v := range copyLabels {
			labels[k] = v
		}
		configMap.Labels = labels

		return ctrl.SetControllerReference(postgresql, configMap, r.Scheme)
	})

	if err != nil {
		return ctrl.Result{}, err
	}

	// Create Persistent Volume to be managed by postgresql

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: "postgres-pv-claim",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
		},
	}

	parseQuantity, err := resource.ParseQuantity(*postgresql.Spec.Storage.Size)
	pvc.Spec.Resources.Requests = corev1.ResourceList{
		corev1.ResourceStorage: parseQuantity,
	}
	_, err = ctrl.CreateOrUpdate(ctx, r.Client, pvc, func() error {

		copyLabels := postgresql.GetLabels()
		if copyLabels == nil {
			copyLabels = map[string]string{}
		}
		labels := map[string]string{}
		for k, v := range copyLabels {
			labels[k] = v
		}
		pvc.Labels = labels

		return ctrl.SetControllerReference(postgresql, pvc, r.Scheme)
	})

	// Create Deployments to be managed by Postgresql
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "postgres-deployment",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: postgresql.Spec.Replicas,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						corev1.Container{
							Name:  "postgres",
							Image: "postgres" + *postgresql.Spec.Version,
							Ports: []corev1.ContainerPort{
								corev1.ContainerPort{
									ContainerPort: 5432,
								},
							},
							EnvFrom: []corev1.EnvFromSource{
								corev1.EnvFromSource{
									ConfigMapRef: &corev1.ConfigMapEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: configMap.Name,
										},
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								corev1.VolumeMount{
									Name:      "postgredb",
									MountPath: "/var/lib/postgresql/data",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						corev1.Volume{
							Name: "postgredb",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: pvc.Name,
								},
							},
						},
					},
				},
			},
		},
	}

	_, err = ctrl.CreateOrUpdate(ctx, r.Client, dep, func() error {

		copyLabels := postgresql.GetLabels()
		if copyLabels == nil {
			copyLabels = map[string]string{}
		}
		labels := map[string]string{}
		for k, v := range copyLabels {
			labels[k] = v
		}
		pvc.Labels = labels

		return ctrl.SetControllerReference(postgresql, dep, r.Scheme)
	})

	// Generate service object managed by Postgres
	svc := &corev1.Service{
		ObjectMeta: ctrl.ObjectMeta{
			Name: "postgresql",
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
			Ports: []corev1.ServicePort{
				corev1.ServicePort{
					Port: 5432,
				},
			},
			Selector: map[string]string{
				"app": "postgres",
			},
		},
	}

	// either create or update the service as appropriate
	_, err = ctrl.CreateOrUpdate(ctx, r.Client, svc, func() error {

		copyLabels := postgresql.GetLabels()
		if copyLabels == nil {
			copyLabels = map[string]string{}
		}
		labels := map[string]string{}
		for k, v := range copyLabels {
			labels[k] = v
		}
		svc.Labels = labels
		return ctrl.SetControllerReference(postgresql, svc, r.Scheme)
	})

	return ctrl.Result{}, err
}

func (r *PostgreSQLReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&databasev1alpha1.PostgreSQL{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&corev1.Service{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}
