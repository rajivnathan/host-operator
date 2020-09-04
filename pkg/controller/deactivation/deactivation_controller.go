package deactivation

import (
	"context"
	"fmt"
	"strconv"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	coputil "github.com/redhat-cop/operator-utils/pkg/util"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_deactivation")

const defaultRequeueTime = time.Minute * 1

// Add creates a new Deactivation Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, _ *configuration.Config) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileDeactivation{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("deactivation-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource MasterUserRecord
	err = c.Watch(&source.Kind{Type: &toolchainv1alpha1.MasterUserRecord{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileDeactivation{}

// ReconcileDeactivation reconciles a Deactivation object
type ReconcileDeactivation struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads the state of the cluster for a Deactivation object and makes changes based on the state read
// and what is in the Deactivation.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileDeactivation) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	logger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	logger.Info("Reconciling Deactivation")

	trigger := &v1.ConfigMap{}
	err := r.client.Get(context.TODO(), types.NamespacedName{
		Namespace: request.Namespace,
		Name:      "start-user-deactivation",
	}, trigger)

	if err == nil {
		log.Info("requeue for a long time")
		// just requeue for a long time
		requeueForALongTime := 24 * time.Hour
		return reconcile.Result{RequeueAfter: requeueForALongTime}, nil
	}
	log.Info("triggered user deactivation")

	// Fetch the MasterUserRecord instance
	mur := &toolchainv1alpha1.MasterUserRecord{}
	err = r.client.Get(context.TODO(), request.NamespacedName, mur)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "unable to get MasterUserRecord")
		return reconcile.Result{}, err
	}

	// If the UserAccount is being deleted, no need to do anything else
	if coputil.IsBeingDeleted(mur) {
		return reconcile.Result{}, nil
	}

	logger.Info("Checking whether the user should be deactivated")
	if len(mur.Spec.UserAccounts) == 0 {
		logger.Error(err, "the mur has no associated user accounts")
		return reconcile.Result{}, err
	}

	// The tier a mur belongs to is defined as part of its user account, since murs can in theory have multiple user accounts we will only consider the first one
	account := mur.Spec.UserAccounts[0]

	nsTemplateTier := &toolchainv1alpha1.NSTemplateTier{}
	tierName := types.NamespacedName{Namespace: request.Namespace, Name: account.Spec.NSTemplateSet.TierName}
	if err := r.client.Get(context.TODO(), tierName, nsTemplateTier); err != nil {
		logger.Error(err, "unable to get NSTemplateTier with name %s", account.Spec.NSTemplateSet.TierName)
		return reconcile.Result{}, err
	}

	// if nsTemplateTier.Spec.DeactivationTimeoutDays == 0 {
	// 	logger.Info("User belongs to a tier that does not have a deactivation timeout. The user will not be automatically deactivated")
	// 	// This tier has no deactivation timeout. Users belonging to this tier will not be auto deactivated, no need to requeue.
	// 	return reconcile.Result{}, nil
	// }

	// If the the mur hasn't been provisioned yet then requeue after the default requeue time
	if mur.Status.ProvisionedTime == nil {
		return reconcile.Result{RequeueAfter: defaultRequeueTime}, nil
	}

	provisionedTimestamp := *mur.Status.ProvisionedTime
	// timeoutDays := nsTemplateTier.Spec.DeactivationTimeoutDays
	// timeoutDuration, err := time.ParseDuration(strconv.Itoa(timeoutDays) + "d")
	timeoutDuration := 1 * time.Second
	// if err != nil {
	// 	// invalid deactivation timeout
	// 	logger.Error(err, "tier %s has an invalid deactivation timeout", account.Spec.NSTemplateSet.TierName)
	// 	return reconcile.Result{}, nil
	// }

	deactivationTimestamp := provisionedTimestamp.Add(timeoutDuration)

	currentTime := time.Now()
	if currentTime.Before(deactivationTimestamp) {
		// it is not yet time to deactivate so requeue when it will be
		requeueAfterExpired := deactivationTimestamp.Sub(currentTime)
		logger.Info("Not yet time to deactivate the user")
		return reconcile.Result{RequeueAfter: requeueAfterExpired}, nil
	}

	logger.Info("Time to deactivate the user")

	// deactivate the user
	usersignup := &toolchainv1alpha1.UserSignup{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{
		Namespace: mur.Namespace,
		Name:      mur.Spec.UserID,
	}, usersignup); err != nil {
		// Error getting usersignup - requeue the request.
		return reconcile.Result{}, err
	}

	usersignup.Spec.Deactivated = true

	if err := r.client.Update(context.TODO(), usersignup); err != nil {
		logger.Error(err, "failed to update usersignup")
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil

	// deactivationUnixTime := provisionedTimestamp.Unix() + (timeoutDays * secondsPerDay)

	// fmt.Printf("rajiv: listed MasterUserRecords count %v\n", len(murs.Items))
	// if len(murs.Items) == 0 {
	// 	fmt.Printf("rajiv: No MURS found")
	// 	return reconcile.Result{RequeueAfter: defaultRequeueTime}, err
	// }

	// Fetch the MURs and deprovision them
	// instance := &toolchainv1alpha1.MasterUserRecord{}
	// err = r.client.Get(context.TODO(), request.NamespacedName, instance)
	// if err != nil {
	// 	if errors.IsNotFound(err) {
	// 		// Request object not found, could have been deleted after reconcile request.
	// 		// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
	// 		// Return and don't requeue
	// 		return reconcile.Result{}, nil
	// 	}
	// 	// Error reading the object - requeue the request.
	// 	return reconcile.Result{RequeueAfter: defaultRequeueTime}, err
	// }
	// reqLogger = reqLogger.WithValues("name", instance.GetName())

	// // Get the deactivation timeout from the instance
	// provisionedTime, err := getProvisionedTime(instance)
	// if err != nil {
	// 	return reconcile.Result{RequeueAfter: defaultRequeueTime}, err
	// }

	// if time.Now().After(time.Unix(provisionedTime, 0)) {
	// 	fmt.Println("current time is after provisioned time")
	// }

	// lengthDuration, err := time.ParseDuration(length)
	// if err != nil || lengthDuration.Minutes() < 0 {
	// 	// the deactivation length is invalid, it must be formatted as a duration string and must be greater than zero (eg. "300ms", "1.5h" or "2h45m". Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h".)
	// 	return reconcile.Result{}, err
	// }
}

func getProvisionedTime(mur *toolchainv1alpha1.MasterUserRecord) (int64, error) {
	if mur.Labels == nil {
		return 0, fmt.Errorf("mur does not have any labels")
	}
	if value, ok := mur.Labels[toolchainv1alpha1.MasterUserRecordProvisionedTimeUNIXLabelKey]; ok {
		provisionedTime, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("mur does not have a valid value for the %s label, value: %s", toolchainv1alpha1.MasterUserRecordProvisionedTimeUNIXLabelKey, value)
		}
		return provisionedTime, nil
	}
	return 0, fmt.Errorf("mur does not have %s label", toolchainv1alpha1.MasterUserRecordProvisionedTimeUNIXLabelKey)
}

func murSelector(timeoutLimit int64) (client.MatchingLabelsSelector, error) {

	selector := labels.NewSelector()
	provisionedTimeLabel, err := labels.NewRequirement(toolchainv1alpha1.MasterUserRecordProvisionedTimeUNIXLabelKey, selection.GreaterThan, []string{"1597341448"})
	if err != nil {
		return client.MatchingLabelsSelector{}, err
	}
	selector = selector.Add(*provisionedTimeLabel)

	return client.MatchingLabelsSelector{
		Selector: selector,
	}, nil
}
