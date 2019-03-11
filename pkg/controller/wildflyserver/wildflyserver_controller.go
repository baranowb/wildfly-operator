package wildflyserver

import (
	"context"
	"reflect"
	"strconv"
	"strings"

	wildflyv1alpha1 "github.com/wildfly/wildfly-operator/pkg/apis/wildfly/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_wildflyserver")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new WildFlyServer Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileWildFlyServer{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("wildflyserver-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource WildFlyServer
	err = c.Watch(&source.Kind{Type: &wildflyv1alpha1.WildFlyServer{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner WildFlyServer
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &wildflyv1alpha1.WildFlyServer{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileWildFlyServer{}

// ReconcileWildFlyServer reconciles a WildFlyServer object
type ReconcileWildFlyServer struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a WildFlyServer object and makes changes based on the state read
// and what is in the WildFlyServer.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileWildFlyServer) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling WildFlyServer")

	// Fetch the WildFlyServer instance
	wildflyServer := &wildflyv1alpha1.WildFlyServer{}
	err := r.client.Get(context.TODO(), request.NamespacedName, wildflyServer)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Check if the deployment already exists, if not create a new one
	foundDeployment := &appsv1.Deployment{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: wildflyServer.Name, Namespace: wildflyServer.Namespace}, foundDeployment)
	if err != nil && errors.IsNotFound(err) {
		// Define a new deployment
		dep := r.deploymentForWildFly(wildflyServer)
		reqLogger.Info("Creating a new Deployment.", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		err = r.client.Create(context.TODO(), dep)
		if err != nil {
			reqLogger.Error(err, "Failed to create new Deployment.", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			return reconcile.Result{}, err
		}
		// Deployment created successfully - return and requeue
		return reconcile.Result{Requeue: true}, nil
	} else if err != nil {
		reqLogger.Error(err, "Failed to get Deployment.")
		return reconcile.Result{}, err
	}

	// Ensure the deployment size is the same as the spec
	size := wildflyServer.Spec.Size
	if *foundDeployment.Spec.Replicas != size {
		reqLogger.Info("Updating replica size to "+strconv.Itoa(int(size)), "Deployment.Namespace", foundDeployment.Namespace, "Deployment.Name", foundDeployment.Name)
		foundDeployment.Spec.Replicas = &size
		err = r.client.Update(context.TODO(), foundDeployment)
		if err != nil {
			reqLogger.Error(err, "Failed to update Deployment.", "Deployment.Namespace", foundDeployment.Namespace, "Deployment.Name", foundDeployment.Name)
			return reconcile.Result{}, err
		}

		// Spec updated - return and requeue
		return reconcile.Result{Requeue: true}, nil
	}

	// Update the WildFlyServer status with the pod names
	// List the pods for this WildFlyServer's deployment
	podList := &corev1.PodList{}
	labelSelector := labels.SelectorFromSet(labelsForWildFly(wildflyServer))
	listOps := &client.ListOptions{
		Namespace:     wildflyServer.Namespace,
		LabelSelector: labelSelector,
	}
	err = r.client.List(context.TODO(), listOps, podList)
	if err != nil {
		reqLogger.Error(err, "Failed to list pods.", "WildFlyServer.Namespace", wildflyServer.Namespace, "WildFlyServer.Name", wildflyServer.Name)
		return reconcile.Result{}, err
	}
	podNames := getPodNames(podList.Items)
	if len(podNames) != int(size) {
		reqLogger.Info("Updating pod names: " + strings.Join(podNames, ", "))
		return reconcile.Result{Requeue: true}, nil
	}
	// Update status.Nodes if needed
	if !reflect.DeepEqual(podNames, wildflyServer.Status.Nodes) {
		wildflyServer.Status.Nodes = podNames
		err := r.client.Status().Update(context.TODO(), wildflyServer)
		if err != nil {
			reqLogger.Error(err, "Failed to update WildFlyServer status.")
			return reconcile.Result{}, err
		}
	}

	// Check if the deployment already exists, if not create a new one
	foundLoadBalancer := &corev1.Service{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: wildflyServer.Name + "-loadbalancer", Namespace: wildflyServer.Namespace}, foundLoadBalancer)
	if err != nil && errors.IsNotFound(err) {
		// Define a new deployment
		loadBalancer := r.loadBalancerForWildFly(wildflyServer)
		reqLogger.Info("Creating a new LoadBalancer.", "LoadBalancer.Namespace", loadBalancer.Namespace, "LoadBalancer.Name", loadBalancer.Name)
		err = r.client.Create(context.TODO(), loadBalancer)
		if err != nil {
			reqLogger.Error(err, "Failed to create new LoadBalancer.", "LoadBalancer.Namespace", loadBalancer.Namespace, "LoadBalancer.Name", loadBalancer.Name)
			return reconcile.Result{}, err
		}
		// Deployment created successfully - return and requeue
		return reconcile.Result{Requeue: true}, nil
	} else if err != nil {
		reqLogger.Error(err, "Failed to get LoadBalancer.")
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

// deploymentForWildFly returns a wildfly Deployment object
func (r *ReconcileWildFlyServer) deploymentForWildFly(w *wildflyv1alpha1.WildFlyServer) *appsv1.Deployment {
	ls := labelsForWildFly(w)
	replicas := w.Spec.Size
	applicationImage := w.Spec.ApplicationImage

	dep := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      w.Name,
			Namespace: w.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  w.Name,
						Image: applicationImage,
						Ports: []corev1.ContainerPort{{
							ContainerPort: 8080,
							Name:          "http",
						}},
					}},
				},
			},
		},
	}
	// Set WildFlyServer instance as the owner and controller
	controllerutil.SetControllerReference(w, dep, r.scheme)
	return dep
}

// loadBalancerForWildFly returns a loadBalancer service
func (r *ReconcileWildFlyServer) loadBalancerForWildFly(w *wildflyv1alpha1.WildFlyServer) *corev1.Service {
	labels := labelsForWildFly(w)
	loadBalancer := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      w.Name + "-loadbalancer",
			Namespace: w.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeLoadBalancer,
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name: "http",
					Port: 8080,
				},
			},
		},
	}
	// Set WildFlyServer instance as the owner and controller
	controllerutil.SetControllerReference(w, loadBalancer, r.scheme)
	return loadBalancer
}

// getPodNames returns the pod names of the array of pods passed in
func getPodNames(pods []corev1.Pod) []string {
	var podNames []string
	for _, pod := range pods {
		podNames = append(podNames, pod.Name)
	}
	return podNames
}

func labelsForWildFly(w *wildflyv1alpha1.WildFlyServer) map[string]string {
	labels := make(map[string]string)
	labels["app"] = w.Name
	if w.Labels != nil {
		for labelKey, labelValue := range w.Labels {
			labels[labelKey] = labelValue
		}
	}
	return labels
}