package toolchainstatus

import (
	"github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	commonclient "github.com/codeready-toolchain/toolchain-common/pkg/client"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateOrUpdateResources(client client.Client, s *runtime.Scheme, namespace string) error {
	toolchainStatus := &v1alpha1.ToolchainStatus{
		ObjectMeta: v1.ObjectMeta{
			Namespace: namespace,
			Name:      "toolchain-status",
		},
		Spec: v1alpha1.ToolchainStatusSpec{},
	}
	commonclient := commonclient.NewApplyClient(client, s)
	_, err := commonclient.CreateOrUpdateObject(toolchainStatus, false, nil)
	return err
}
