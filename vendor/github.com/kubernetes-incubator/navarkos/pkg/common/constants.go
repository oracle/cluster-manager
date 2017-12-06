package common

const (
	NavarkosClusterStateKey                            = "n6s.io/cluster.lifecycle.state"
	NavarkosClusterStateOffline                        = "offline"
	NavarkosClusterStatePending                        = "pending-provision"
	NavarkosClusterStateReady                          = "ready"
	NavarkosClusterStatePendingShutdown                = "pending-shutdown"
	NavarkosClusterStatePendingScaleUp                 = "pending-up"
	NavarkosClusterStatePendingScaleDown               = "pending-down"
	NavarkosClusterStateProvisioning                   = "provisioning"
	NavarkosClusterStateJoining                        = "joining"
	NavarkosClusterStateShuttingDown                   = "shutting-down"
	NavarkosClusterStateScalingUp                      = "scaling-up"
	NavarkosClusterStateScalingDown                    = "scaling-down"
	NavarkosClusterStateFailedProvision                = "failed-provision"
	NavarkosClusterStateFailedScaleUp                  = "failed-up"
	NavarkosClusterStateFailedScaleDown                = "failed-down"
	NavarkosClusterCapacityAllocatablePodsKey          = "n6s.io/cluster.capacity.allocatable-pods"
	NavarkosClusterCapacityPodsKey                     = "n6s.io/cluster.capacity.total-capacity-pods"
	NavarkosClusterCapacityUsedPodsKey                 = "n6s.io/cluster.capacity.used-pods"
	NavarkosClusterCapacitySystemPodsKey               = "n6s.io/cluster.capacity.used-system-pods"
	NavarkosClusterPriorityKey                         = "n6s.io/cluster.priority"
	NavarkosClusterDefaultPriority                 int = 1
	NavarkosClusterShutdownStartTimeKey                = "n6s.io/cluster.idle-start-timestamp"
	NavarkosClusterTimeToLiveBeforeShutdownKey         = "n6s.io/cluster.idle-time-to-live" // if set to a value of less than or equal 0, it will disable this
	NavarkosDefaultClusterTimeToLiveBeforeShutdown     = 1200                               // 20 minutes - default value in seconds for cluster to be empty to trigger shutdown
	NavarkosClusterAutoScaleKey                        = "n6s.io/cluster.autoscale.enabled"
	NavarkosClusterScaleUpCapacityThresholdKey         = "n6s.io/cluster.autoscale.scale-up-threshold"
	NavarkosDefaultClusterScaleUpCapacityThreshold     = 80 // default value in Percentage to trigger cluster scale up
	NavarkosClusterScaleDownCapacityThresholdKey       = "n6s.io/cluster.autoscale.scale-down-threshold"
	NavarkosClusterScaleDownCapacityThreshold          = 10 // default value in Percentage to trigger cluster scale down
	NavarkosClusterNodeCountKey                        = "n6s.io/cluster.node-count"
)
