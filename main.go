package main

import (
	"context"
	"fmt"
	"os"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// WebApp represents our custom resource
type WebApp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              WebAppSpec   `json:"spec,omitempty"`
	Status            WebAppStatus `json:"status,omitempty"`
}

type WebAppSpec struct {
	Image    string       `json:"image"`
	Replicas int32        `json:"replicas"`
	Port     int32        `json:"port,omitempty"`
	Env      []EnvVar     `json:"env,omitempty"`
}

type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type WebAppStatus struct {
	AvailableReplicas int32              `json:"availableReplicas,omitempty"`
	Conditions        []metav1.Condition `json:"conditions,omitempty"`
}

type WebAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WebApp `json:"items"`
}

// Implement required methods for runtime.Object
func (w *WebApp) DeepCopyObject() runtime.Object {
	return w.DeepCopy()
}

func (w *WebApp) DeepCopy() *WebApp {
	if w == nil {
		return nil
	}
	out := new(WebApp)
	w.DeepCopyInto(out)
	return out
}

func (w *WebApp) DeepCopyInto(out *WebApp) {
	*out = *w
	out.TypeMeta = w.TypeMeta
	w.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = w.Spec
	out.Status = w.Status
}

func (w *WebAppList) DeepCopyObject() runtime.Object {
	return w.DeepCopy()
}

func (w *WebAppList) DeepCopy() *WebAppList {
	if w == nil {
		return nil
	}
	out := new(WebAppList)
	w.DeepCopyInto(out)
	return out
}

func (w *WebAppList) DeepCopyInto(out *WebAppList) {
	*out = *w
	out.TypeMeta = w.TypeMeta
	w.ListMeta.DeepCopyInto(&out.ListMeta)
	if w.Items != nil {
		out.Items = make([]WebApp, len(w.Items))
		for i := range w.Items {
			w.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

// WebAppReconciler reconciles a WebApp object
type WebAppReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile is the core reconciliation loop
// This is where the "magic" happens - it ensures actual state matches desired state
func (r *WebAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Starting reconciliation", "webapp", req.NamespacedName)

	// STEP 1: Fetch the WebApp custom resource
	webapp := &WebApp{}
	err := r.Get(ctx, req.NamespacedName, webapp)
	if err != nil {
		if errors.IsNotFound(err) {
			// Resource deleted - nothing to do
			log.Info("WebApp resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get WebApp")
		return ctrl.Result{}, err
	}

	// STEP 2: Check if Deployment exists, create if not
	deployment := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: webapp.Name, Namespace: webapp.Namespace}, deployment)
	if err != nil && errors.IsNotFound(err) {
		// Create new Deployment
		dep := r.deploymentForWebApp(webapp)
		log.Info("Creating a new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		err = r.Create(ctx, dep)
		if err != nil {
			log.Error(err, "Failed to create new Deployment")
			return ctrl.Result{}, err
		}
		// Deployment created successfully - requeue to update status
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Deployment")
		return ctrl.Result{}, err
	}

	// STEP 3: Ensure Deployment matches the spec (reconcile drift)
	if *deployment.Spec.Replicas != webapp.Spec.Replicas {
		log.Info("Deployment replicas do not match spec, updating",
			"current", *deployment.Spec.Replicas,
			"desired", webapp.Spec.Replicas)
		deployment.Spec.Replicas = &webapp.Spec.Replicas
		err = r.Update(ctx, deployment)
		if err != nil {
			log.Error(err, "Failed to update Deployment")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// STEP 4: Check if Service exists, create if not
	service := &corev1.Service{}
	err = r.Get(ctx, types.NamespacedName{Name: webapp.Name, Namespace: webapp.Namespace}, service)
	if err != nil && errors.IsNotFound(err) {
		// Create new Service
		svc := r.serviceForWebApp(webapp)
		log.Info("Creating a new Service", "Service.Namespace", svc.Namespace, "Service.Name", svc.Name)
		err = r.Create(ctx, svc)
		if err != nil {
			log.Error(err, "Failed to create new Service")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Service")
		return ctrl.Result{}, err
	}

	// STEP 5: Update status
	webapp.Status.AvailableReplicas = deployment.Status.AvailableReplicas

	// Update conditions
	condition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "DeploymentReady",
		Message:            fmt.Sprintf("Deployment has %d/%d replicas available", deployment.Status.AvailableReplicas, webapp.Spec.Replicas),
		LastTransitionTime: metav1.Now(),
	}

	if deployment.Status.AvailableReplicas != webapp.Spec.Replicas {
		condition.Status = metav1.ConditionFalse
		condition.Reason = "DeploymentNotReady"
	}

	webapp.Status.Conditions = []metav1.Condition{condition}

	err = r.Status().Update(ctx, webapp)
	if err != nil {
		log.Error(err, "Failed to update WebApp status")
		return ctrl.Result{}, err
	}

	log.Info("Reconciliation complete", "availableReplicas", webapp.Status.AvailableReplicas)

	// Requeue after 30 seconds to check status periodically
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// deploymentForWebApp creates a Deployment from the WebApp spec
func (r *WebAppReconciler) deploymentForWebApp(webapp *WebApp) *appsv1.Deployment {
	labels := map[string]string{
		"app":        webapp.Name,
		"managed-by": "webapp-operator",
	}

	replicas := webapp.Spec.Replicas
	port := webapp.Spec.Port
	if port == 0 {
		port = 8080
	}

	// Convert our EnvVar type to Kubernetes EnvVar type
	envVars := []corev1.EnvVar{}
	for _, env := range webapp.Spec.Env {
		envVars = append(envVars, corev1.EnvVar{
			Name:  env.Name,
			Value: env.Value,
		})
	}

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      webapp.Name,
			Namespace: webapp.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  webapp.Name,
						Image: webapp.Spec.Image,
						Ports: []corev1.ContainerPort{{
							ContainerPort: port,
							Name:          "http",
						}},
						Env: envVars,
					}},
				},
			},
		},
	}

	// Set WebApp instance as the owner and controller
	controllerutil.SetControllerReference(webapp, dep, r.Scheme)
	return dep
}

// serviceForWebApp creates a Service for the WebApp
func (r *WebAppReconciler) serviceForWebApp(webapp *WebApp) *corev1.Service {
	labels := map[string]string{
		"app":        webapp.Name,
		"managed-by": "webapp-operator",
	}

	port := webapp.Spec.Port
	if port == 0 {
		port = 8080
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      webapp.Name,
			Namespace: webapp.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{{
				Port:       80,
				TargetPort: intstr.FromInt(int(port)),
				Protocol:   corev1.ProtocolTCP,
				Name:       "http",
			}},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	controllerutil.SetControllerReference(webapp, svc, r.Scheme)
	return svc
}

// SetupWithManager sets up the controller with the Manager
func (r *WebAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&WebApp{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

func main() {
	// Setup logger
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	setupLog := ctrl.Log.WithName("setup")

	// Create manager
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: runtime.NewScheme(),
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Register our types
	schemeBuilder := runtime.NewSchemeBuilder(func(scheme *runtime.Scheme) error {
		scheme.AddKnownTypes(
			metav1.SchemeGroupVersion.WithGroup("example.com").WithVersion("v1"),
			&WebApp{},
			&WebAppList{},
		)
		metav1.AddToGroupVersion(scheme, metav1.SchemeGroupVersion.WithGroup("example.com").WithVersion("v1"))
		return nil
	})

	if err := schemeBuilder.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "unable to add scheme")
		os.Exit(1)
	}

	// Add standard Kubernetes types
	if err := appsv1.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "unable to add appsv1 to scheme")
		os.Exit(1)
	}
	if err := corev1.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "unable to add corev1 to scheme")
		os.Exit(1)
	}

	// Setup reconciler
	if err = (&WebAppReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "WebApp")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
