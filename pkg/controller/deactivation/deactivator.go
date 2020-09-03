package deactivation

import (
	"context"
	"fmt"
	"strconv"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/go-logr/logr"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	"sigs.k8s.io/controller-runtime/pkg/client"
	clientlib "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	v1 "k8s.io/api/core/v1"
)

var log = logf.Log.WithName("deactivator")

func StartDeactivator(mgr manager.Manager, namespace string, stopChan <-chan struct{}, period time.Duration) {
	log.Info("starting deactivator", "period", period)
	go wait.Until(func() {
		deactivator := deactivator{
			client:    mgr.GetClient(),
			namespace: namespace,
			logger:    log.WithValues("Namespace", namespace),
		}
		deactivator.checkUserRecordsForDeactivation()
	}, period, stopChan)
}

type deactivator struct {
	client    client.Client
	namespace string
	logger    logr.Logger
}

func (d deactivator) checkUserRecordsForDeactivation() {

	d.logger.Info("listing NSTemplateTier resources")

	tiers := toolchainv1alpha1.NSTemplateTierList{}
	if err := d.client.List(context.Background(), &tiers,
		clientlib.InNamespace(d.namespace),
	); err != nil {
		d.logger.Error(err, "unable to get MasterUserRecords")
	}

	log.Info("listed NSTemplateTiers", "count", len(tiers.Items))
	if len(tiers.Items) == 0 {
		// no tiers to process
		return
	}

	// find MURs that should be deactivated for each tier
	for _, nsTemplateTier := range tiers.Items {
		tier := toolchainv1alpha1.NSTemplateTier{}
		if err := d.client.Get(context.TODO(), types.NamespacedName{
			Namespace: nsTemplateTier.Namespace,
			Name:      nsTemplateTier.Name,
		}, &tier); err != nil {
			d.logger.Error(err, "unable to get NSTemplateTier")
			continue
		}

		d.deactivateExpiredMursForTier(tier)
	} // end tiers loop
}

func (d deactivator) deactivateExpiredMursForTier(tier toolchainv1alpha1.NSTemplateTier) {
	// if tier.Spec.DeactivationTimeoutDays == nil || len(*tier.Spec.DeactivationTimeoutDays) == 0 {
	// 	continue
	// }

	// // timeoutDuration, err := time.ParseDuration(*tier.Spec.DeactivationTimeoutDays)
	// timeoutDays, err := strconv.ParseInt(*tier.Spec.DeactivationTimeoutDays, 10, 64)
	// if err != nil {
	// 	return errs.Wrapf(err, "tier %s deactivation timeout value is invalid")
	// }

	// timeoutDuration := time.Duration(timeoutDays*24) * time.Hour

	// TODO remove temporary hard-coded timeout duration value
	timeoutDuration := 60 * time.Minute
	trigger := &v1.ConfigMap{}
	err := d.client.Get(context.TODO(), types.NamespacedName{
		Namespace: d.namespace,
		Name:      "start-user-deactivation",
	}, trigger)

	if err != nil {
		log.Error(err, "unable to get configmap to trigger user deactivation")
	} else {
		timeoutDuration = 60 * time.Second
	}

	expiry := fmt.Sprintf("%d", time.Now().Add(-timeoutDuration).Unix())

	log.Info("masteruserrecord label selector", "expiry", expiry)
	matchingLabels, err := expiredMurSelector(expiry)
	if err != nil {
		d.logger.Error(err, "mur labels invalid")
	}
	murs := toolchainv1alpha1.MasterUserRecordList{}
	if err = d.client.List(context.Background(), &murs,
		clientlib.InNamespace(d.namespace),
		clientlib.Limit(5),
		matchingLabels,
	); err != nil {
		d.logger.Error(err, "unable to list masteruserrecords")
	}
	log.Info("rajiv: masteruserrecords to deactivate", "count", len(murs.Items))

	for _, mur := range murs.Items {
		usersignup := &toolchainv1alpha1.UserSignup{}
		if err := d.client.Get(context.TODO(), types.NamespacedName{
			Namespace: mur.Namespace,
			Name:      mur.Spec.UserID,
		}, usersignup); err != nil {
			log.Error(err, "unable to get usersignup")
			continue
		}

		log.Info("deactivating usersignup for mur", "name", mur.Name, "tier", mur.Spec.UserAccounts[0].Spec.NSTemplateSet.TierName)

		// trigger deactivation on the user signup
		usersignup.Spec.Deactivated = true
		d.client.Update(context.TODO(), usersignup)

	} // end murs loop
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

func expiredMurSelector(expiry string) (client.MatchingLabelsSelector, error) {
	// retrieve all murs provisioned beyond the expiry period, provisioned timestamps less than the expiry timestamp indicate they are too old
	selector := labels.NewSelector()
	provisionedTimeLabel, err := labels.NewRequirement(toolchainv1alpha1.MasterUserRecordProvisionedTimeUNIXLabelKey, selection.LessThan, []string{expiry})
	if err != nil {
		return client.MatchingLabelsSelector{}, err
	}
	selector = selector.Add(*provisionedTimeLabel)

	return client.MatchingLabelsSelector{
		Selector: selector,
	}, nil
}
