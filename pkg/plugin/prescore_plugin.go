package plugin

import (
	"context"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

var _ framework.PreScorePlugin = &VGPUSchedulerPlugin{}

func (p *VGPUSchedulerPlugin) PreScore(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodes []*framework.NodeInfo) *framework.Status {
	logger := klog.FromContext(ctx)
	if !p.isVGPUResourcePod(state, pod) {
		logger.Info("pod did not request vGPU, skipping node Score", "pod", klog.KObj(pod), "plugin", "PreScore")
		return framework.NewStatus(framework.Skip, "")
	}
	return framework.NewStatus(framework.Success, "")
}
