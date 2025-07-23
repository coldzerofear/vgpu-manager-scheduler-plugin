package plugin

import (
	"context"
	"strings"

	"github.com/coldzerofear/vgpu-manager/pkg/device"
	"github.com/coldzerofear/vgpu-manager/pkg/device/allocator"
	"github.com/coldzerofear/vgpu-manager/pkg/util"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

var _ framework.ScorePlugin = &VGPUSchedulerPlugin{}

func (p *VGPUSchedulerPlugin) Score(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) (int64, *framework.Status) {
	logger := klog.FromContext(ctx)
	score := framework.MinNodeScore
	if !p.isVGPUResourcePod(state, pod) {
		logger.Info("pod did not request vGPU, skipping node Score", "pod", klog.KObj(pod), "plugin", "Score")
		return score, framework.NewStatus(framework.Success, "")
	}
	nodeInfo, err := p.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		errMsg := "getting node from Snapshot failed"
		logger.Error(err, errMsg, "node", nodeName)
		return score, framework.NewStatus(framework.Error, errMsg)
	}
	devNodeInfo, err := p.getDevNodeInfo(state, nodeInfo)
	if err != nil {
		logger.Error(err, "node score calculation failed", "pod", klog.KObj(pod), "node", nodeName)
		return score, framework.NewStatus(framework.Error, err.Error())
	}
	// Sort nodes according to node scheduling strategy.
	nodePolicy, _ := util.HasAnnotation(pod, util.NodeSchedulerPolicyAnnotation)
	switch strings.ToLower(nodePolicy) {
	case string(util.BinpackPolicy):
		klog.V(4).Infof("Pod <%s> use <%s> node scheduling policy", klog.KObj(pod), nodePolicy)
		score = int64(allocator.GetBinpackNodeScore(devNodeInfo, float64(framework.MaxNodeScore)))
	case string(util.SpreadPolicy):
		klog.V(4).Infof("Pod <%s> use <%s> node scheduling policy", klog.KObj(pod), nodePolicy)
		score = int64(allocator.GetSpreadNodeScore(devNodeInfo, float64(framework.MaxNodeScore)))
	default:
		klog.V(4).Infof("Pod <%s> no node scheduling policy", klog.KObj(pod))
		score = 50
	}
	score = addGPUTopologyScore(pod, devNodeInfo, score)
	logger.Info("Calculate node score", "score", score, "node", nodeName)
	return score, framework.NewStatus(framework.Success, "")
}

func addGPUTopologyScore(pod *v1.Pod, nodeInfo *device.NodeInfo, score int64) int64 {
	const topologyAdjustmentPercent = 10
	topoMode, ok := util.HasAnnotation(pod, util.DeviceTopologyModeAnnotation)
	if ok && strings.EqualFold(topoMode, string(util.LinkTopology)) {
		adjustment := (score*topologyAdjustmentPercent + 99) / 100
		if nodeInfo.HasGPUTopology() {
			score += adjustment
			klog.V(4).Infof("Adding %.0f%% topology bonus (%d) for node %s (pod: %s)",
				float64(topologyAdjustmentPercent), adjustment, nodeInfo.GetName(), klog.KObj(pod))
		} else {
			score -= adjustment
			klog.V(4).Infof("Applying %.0f%% topology penalty (%d) for node %s (pod: %s)",
				float64(topologyAdjustmentPercent), adjustment, nodeInfo.GetName(), klog.KObj(pod))
		}
	}
	return clampScore(score)
}

func clampScore(score int64) int64 {
	switch {
	case score > framework.MaxTotalScore:
		klog.V(5).Infof("Clamping score from %d to max %d", score, framework.MaxTotalScore)
		return framework.MaxTotalScore
	case score < framework.MinNodeScore:
		klog.V(5).Infof("Clamping score from %d to min %d", score, framework.MinNodeScore)
		return framework.MinNodeScore
	default:
		return score
	}
}

// ScoreExtensions returns a ScoreExtensions interface if it implements one, or nil if does not.
func (p *VGPUSchedulerPlugin) ScoreExtensions() framework.ScoreExtensions {
	return nil
}
