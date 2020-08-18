package deactivation

import (
	"context"
	"fmt"
	"strconv"
	"time"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	errs "github.com/pkg/errors"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	"sigs.k8s.io/controller-runtime/pkg/client"
	clientlib "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("deactivator")

func StartDeactivator(mgr manager.Manager, namespace string, stopChan <-chan struct{}, period time.Duration) {
	log.Info("starting deactivator", "period", period)
	go wait.Until(func() {
		if err := checkUserRecordsForDeactivation(namespace, mgr.GetClient()); err != nil {
			fmt.Println(err.Error())
		}
	}, period, stopChan)
}

func checkUserRecordsForDeactivation(namespace string, mgrClient clientlib.Client) error {
	checkLogger := log.WithValues("Namespace", namespace)
	checkLogger.Info("listing NSTemplateTier resources")

	tiers := toolchainv1alpha1.NSTemplateTierList{}
	if err := mgrClient.List(context.Background(), &tiers,
		clientlib.InNamespace(namespace),
		clientlib.Limit(5),
	); err != nil {
		return errs.Wrap(err, "unable to get MasterUserRecords")
	}

	log.Info("listed NSTemplateTiers", "count", len(tiers.Items))
	if len(tiers.Items) == 0 {
		// no tiers to process
		return nil
	}

	// find MURs that should be deactivated for each tier
	for _, nsTemplateTier := range tiers.Items {
		murs := toolchainv1alpha1.MasterUserRecordList{}
		tier := toolchainv1alpha1.NSTemplateTier{}
		if err := mgrClient.Get(context.TODO(), types.NamespacedName{
			Namespace: nsTemplateTier.Namespace,
			Name:      nsTemplateTier.Name,
		}, &tier); err != nil {
			return errs.Wrap(err, "unable to get NSTemplateTier")
		}

		// if tier.Spec.DeactivationTimeoutSeconds == nil || len(*tier.Spec.DeactivationTimeoutSeconds) == 0 {
		// 	continue
		// }

		// // timeoutDuration, err := time.ParseDuration(*tier.Spec.DeactivationTimeoutSeconds)
		// timeoutDays, err := strconv.ParseInt(*tier.Spec.DeactivationTimeoutSeconds, 10, 64)
		// if err != nil {
		// 	return errs.Wrapf(err, "tier %s deactivation timeout value is invalid")
		// }

		// timeoutDuration := time.Duration(timeoutDays*24) * time.Hour

		// TODO remove temporary hard-coded timeout duration value
		timeoutDuration := 20 * time.Second
		provisionedBefore := fmt.Sprintf("%d", time.Now().Add(-timeoutDuration).Unix())

		log.Info("masteruserrecord label selector", "provisionedBefore", provisionedBefore)
		matchingLabels, err := murSelector(provisionedBefore)
		if err != nil {
			log.Info("error mur selector")
			return errs.Wrap(err, "unable to get masteruserrecords")
		}
		if err = mgrClient.List(context.Background(), &murs,
			clientlib.InNamespace(namespace),
			clientlib.Limit(5),
			matchingLabels,
		); err != nil {
			log.Info("error mur list")
			return errs.Wrap(err, "unable to get masteruserrecords")
		}
		log.Info("rajiv: masteruserrecords to deactivate", "count", len(murs.Items))

		for _, mur := range murs.Items {
			usersignup := &toolchainv1alpha1.UserSignup{}
			if err := mgrClient.Get(context.TODO(), types.NamespacedName{
				Namespace: mur.Namespace,
				Name:      mur.Spec.UserID,
			}, usersignup); err != nil {
				log.Error(err, "unable to get usersignup")
				continue
			}

			// trigger deactivation on the user signup
			usersignup.Spec.Deactivated = true
			mgrClient.Update(context.TODO(), usersignup)

			log.Info("rajiv: found mur", "name", mur.Name, "tier", mur.Spec.UserAccounts[0].Spec.NSTemplateSet.TierName)

		} // end murs loop

	} // end tiers loop

	return nil
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

func murSelector(provisionedBefore string) (client.MatchingLabelsSelector, error) {
	selector := labels.NewSelector()
	provisionedTimeLabel, err := labels.NewRequirement(toolchainv1alpha1.MasterUserRecordProvisionedTimeUNIXLabelKey, selection.LessThan, []string{provisionedBefore})
	if err != nil {
		return client.MatchingLabelsSelector{}, err
	}
	selector = selector.Add(*provisionedTimeLabel)

	return client.MatchingLabelsSelector{
		Selector: selector,
	}, nil
}
