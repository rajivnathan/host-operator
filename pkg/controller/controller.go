package controller

import (
	"github.com/codeready-toolchain/host-operator/pkg/configuration"
	"github.com/codeready-toolchain/host-operator/pkg/controller/changetierrequest"
	"github.com/codeready-toolchain/host-operator/pkg/controller/masteruserrecord"
	"github.com/codeready-toolchain/host-operator/pkg/controller/notification"
	"github.com/codeready-toolchain/host-operator/pkg/controller/nstemplatetier"
	"github.com/codeready-toolchain/host-operator/pkg/controller/registrationservice"
	"github.com/codeready-toolchain/host-operator/pkg/controller/toolchainstatus"
	"github.com/codeready-toolchain/host-operator/pkg/controller/usersignup"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// addToManagerFuncs is a list of functions to add all Controllers to the Manager
var addToManagerFuncs []func(manager.Manager, *configuration.Config) error

func init() {
	addToManagerFuncs = append(addToManagerFuncs, masteruserrecord.Add)
	addToManagerFuncs = append(addToManagerFuncs, registrationservice.Add)
	addToManagerFuncs = append(addToManagerFuncs, usersignup.Add)
	addToManagerFuncs = append(addToManagerFuncs, changetierrequest.Add)
	addToManagerFuncs = append(addToManagerFuncs, nstemplatetier.Add)
	addToManagerFuncs = append(addToManagerFuncs, notification.Add)
	addToManagerFuncs = append(addToManagerFuncs, toolchainstatus.Add)
}

// AddToManager adds all Controllers to the Manager
func AddToManager(m manager.Manager, config *configuration.Config) error {
	for _, f := range addToManagerFuncs {
		if err := f(m, config); err != nil {
			return err
		}
	}
	return nil
}
