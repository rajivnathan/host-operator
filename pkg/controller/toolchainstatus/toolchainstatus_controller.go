package toolchainstatus

import (
	"context"
	"fmt"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/toolchain-common/pkg/cluster"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"

	"github.com/go-logr/logr"
	errs "github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sigs.k8s.io/kubefed/pkg/controller/util"
)

var log = logf.Log.WithName("controller_toolchainstatus")

// Add creates a new ToolchainStatus Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, _ *configuration.Config) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) *ReconcileToolchainStatus {
	return &ReconcileToolchainStatus{
		client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r *ReconcileToolchainStatus) error {
	// Create a new controller
	c, err := controller.New("toolchainstatus-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource ToolchainStatus
	err = c.Watch(&source.Kind{Type: &toolchainv1alpha1.ToolchainStatus{}}, &handler.EnqueueRequestForObject{}, predicate.GenerationChangedPredicate{})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileToolchainStatus implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileToolchainStatus{}

// ReconcileToolchainStatus reconciles a ToolchainStatus object
type ReconcileToolchainStatus struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads the state of the cluster for a ToolchainStatus object and makes changes based on the state read
// and what is in the ToolchainStatus.Spec
func (r *ReconcileToolchainStatus) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling ToolchainStatus")

	// Fetch the ToolchainStatus
	toolchainStatus := &toolchainv1alpha1.ToolchainStatus{}
	err := r.client.Get(context.TODO(), request.NamespacedName, toolchainStatus)
	if err != nil {
		if errors.IsNotFound(err) {
			// Create ToolchainStatus resource if it does not exist
			if err := CreateOrUpdateResources(r.client, r.scheme, request.NamespacedName.Namespace); err != nil {
				// TODO handle error
				return reconcile.Result{}, err
			}
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Look up status of host deployment
	hostDeploymentName := types.NamespacedName{Namespace: request.Namespace, Name: "host-operator"}
	hostDeployment := &appsv1.Deployment{}
	err = r.client.Get(context.TODO(), hostDeploymentName, hostDeployment)
	if err != nil {
		// host deployment not found
		// r.wrapErrorWithStatusUpdate(reqLogger, toolchainStatus, r.setStatusFailed(toolchainv1alpha1.RegistrationServiceDeployingFailedReason), err, "host deployment not found, this should never happen")
		// TODO handle error
	} else {
		reqLogger.Info("Retrieved host deployment: " + hostDeployment.Name)
		for _, condition := range hostDeployment.Status.Conditions {
			reqLogger.Info(fmt.Sprintf("host deployment %s: %s", condition.Type, condition.Status))
		}
	}

	// Look up registration service deployment

	// Look up member statuses
	memberStatuses, err := r.getMemberStatuses()
	if err != nil {
		// TODO handle error
	}

	r.updateStatusConditions(toolchainStatus, memberStatuses, toBeDeployed())

	reqLogger.Info("Finished updating the toolchain status, requeueing...")
	return reconcile.Result{RequeueAfter: time.Second * 10}, nil
}

func (r *ReconcileToolchainStatus) getMemberStatuses() ([]toolchainv1alpha1.MemberStatus, error) {
	// get member statuses from member clusters
	memberClusters := cluster.GetMemberClusters()
	memberStatuses := []toolchainv1alpha1.MemberStatus{}
	for _, memberCluster := range memberClusters {
		nsdName := types.NamespacedName{Namespace: memberCluster.OperatorNamespace, Name: "toolchain-member-status"}
		memberStatus := &toolchainv1alpha1.MemberStatus{}
		if err := memberCluster.Client.Get(context.TODO(), nsdName, memberStatus); err != nil {
			// TODO handle error
		}
		memberStatuses = append(memberStatuses, *memberStatus)
	}
	return memberStatuses, nil
}

func (r *ReconcileToolchainStatus) getMemberCluster(targetCluster string) (*cluster.FedCluster, error) {
	// get & check fed cluster
	fedCluster, ok := cluster.GetFedCluster(targetCluster)
	if !ok {
		return nil, fmt.Errorf("the member cluster %s not found in the registry", targetCluster)
	}
	if !util.IsClusterReady(fedCluster.ClusterStatus) {
		return nil, fmt.Errorf("the member cluster %s is not ready", targetCluster)
	}
	return fedCluster, nil
}

// updateStatusConditions updates RegistrationService status conditions with the new conditions
func (r *ReconcileToolchainStatus) updateStatusConditions(toolchainStatus *toolchainv1alpha1.ToolchainStatus, memberStatuses []toolchainv1alpha1.MemberStatus, newConditions ...toolchainv1alpha1.Condition) error {
	var updated bool
	toolchainStatus.Status.MemberStatus = memberStatuses
	toolchainStatus.Status.Conditions, updated = condition.AddOrUpdateStatusConditions(toolchainStatus.Status.Conditions, newConditions...)
	if !updated {
		// Nothing changed
		return nil
	}
	return r.client.Status().Update(context.TODO(), toolchainStatus)
}

// func (r *ReconcileToolchainStatus) setStatusFailed(reason string) func(toolchainStatus *toolchainv1alpha1.ToolchainStatus, message string) error {
// 	return func(toolchainStatus *toolchainv1alpha1.ToolchainStatus, message string) error {
// 		return r.updateStatusConditions(
// 			toolchainStatus,
// 			toBeNotReady(reason, message))
// 	}
// }

// wrapErrorWithStatusUpdate wraps the error and update the ToolchainStatus status. If the update fails then the error is logged.
func (r *ReconcileToolchainStatus) wrapErrorWithStatusUpdate(logger logr.Logger, toolchainStatus *toolchainv1alpha1.ToolchainStatus,
	statusUpdater func(toolchainStatus *toolchainv1alpha1.ToolchainStatus, message string) error, err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	if err := statusUpdater(toolchainStatus, err.Error()); err != nil {
		logger.Error(err, "Error updating ToolchainStatus status")
	}
	return errs.Wrapf(err, format, args...)
}

func toBeDeployed() toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:   toolchainv1alpha1.ConditionReady,
		Status: corev1.ConditionTrue,
		Reason: fmt.Sprintf("Last updated: %s", time.Now()),
	}
}

func toBeNotReady(reason, msg string) toolchainv1alpha1.Condition {
	return toolchainv1alpha1.Condition{
		Type:    toolchainv1alpha1.ConditionReady,
		Status:  corev1.ConditionFalse,
		Reason:  reason,
		Message: msg,
	}
}
