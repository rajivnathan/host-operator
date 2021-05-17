package toolchainconfig

import "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"

const (
	defaultAutomaticApprovalEnabled                          = false
	defaultAutomaticApprovalResourceCapacityThresholdDefault = 0
	defaultAutomaticApprovalMaxNumberOfUsersOverall          = 0
)

type Config struct {
	toolchainconfig *v1alpha1.ToolchainConfigSpec
}

func (c *Config) AutomaticApprovalEnabled() bool {
	return getBool(c.toolchainconfig.Host.AutomaticApproval.Enabled, defaultAutomaticApprovalEnabled)
}

func (c *Config) AutomaticApprovalResourceCapacityThresholdDefault() int {
	return getInt(c.toolchainconfig.Host.AutomaticApproval.ResourceCapacityThreshold.DefaultThreshold, defaultAutomaticApprovalResourceCapacityThresholdDefault)
}

func (c *Config) AutomaticApprovalResourceCapacityThresholdSpecificPerMemberCluster() map[string]int {
	return c.toolchainconfig.Host.AutomaticApproval.ResourceCapacityThreshold.SpecificPerMemberCluster
}

func (c *Config) AutomaticApprovalMaxNumberOfUsersOverall() int {
	return getInt(c.toolchainconfig.Host.AutomaticApproval.MaxNumberOfUsers.Overall, defaultAutomaticApprovalMaxNumberOfUsersOverall)
}

func (c *Config) AutomaticApprovalMaxNumberOfUsersSpecificPerMemberCluster() map[string]int {
	return c.toolchainconfig.Host.AutomaticApproval.MaxNumberOfUsers.SpecificPerMemberCluster
}

func (c *Config) DeactivationDeactivatingNotificationDays() int {
	return getInt(c.toolchainconfig.Host.AutomaticApproval.MaxNumberOfUsers.Overall, 3)
}

func getBool(value *bool, defaultValue bool) bool {
	if value != nil {
		return *value
	}
	return defaultValue
}

func getInt(value *int, defaultValue int) int {
	if value != nil {
		return *value
	}
	return defaultValue
}

func getString(value *string, defaultValue string) string {
	if value != nil {
		return *value
	}
	return defaultValue
}
