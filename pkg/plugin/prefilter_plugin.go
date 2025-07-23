package plugin

import (
	"context"
	"fmt"

	"github.com/coldzerofear/vgpu-manager/pkg/util"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

var _ framework.PreFilterPlugin = &VGPUSchedulerPlugin{}

func (p *VGPUSchedulerPlugin) PreFilter(ctx context.Context, state *framework.CycleState, pod *v1.Pod) (*framework.PreFilterResult, *framework.Status) {
	logger := klog.FromContext(ctx)
	if !p.isVGPUResourcePod(state, pod) {
		logger.Info("pod did not request vGPU, skipping device filtering", "pod", klog.KObj(pod))
		return nil, framework.NewStatus(framework.Skip, "")
	}
	if err := p.checkDeviceRequests(pod); err != nil {
		logger.Error(err, "check device requests failed", "pod", klog.KObj(pod))
		p.handle.EventRecorder().Eventf(pod, nil, v1.EventTypeWarning, "FailedFiltering", "Scheduling", err.Error())
		return nil, framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
	}
	return nil, framework.NewStatus(framework.Success, "")
}

func (p *VGPUSchedulerPlugin) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}

func (p *VGPUSchedulerPlugin) checkDeviceRequests(pod *v1.Pod) error {
	for _, container := range pod.Spec.Containers {
		if util.GetResourceOfContainer(&container, util.VGPUCoreResourceName) > util.HundredCore {
			return fmt.Errorf("container %s requests vGPU core exceeding limit, maxLimit: %d", container.Name, util.HundredCore)
		}
		if util.GetResourceOfContainer(&container, util.VGPUNumberResourceName) > util.MaxDeviceNumber {
			return fmt.Errorf("container %s requests vGPU number exceeding limit, maxLimit: %d", container.Name, util.MaxDeviceNumber)
		}
	}
	return nil
}

func (p *VGPUSchedulerPlugin) getTotalRequestVGPUByPod(state *framework.CycleState, pod *v1.Pod) int {
	totalVGPU, err := state.Read(util.VGPUNumberResourceName)
	if err != nil {
		totalNum := util.GetResourceOfPod(pod, util.VGPUNumberResourceName)
		totalVGPU = resourceNumber(totalNum)
		state.Write(util.VGPUNumberResourceName, totalVGPU)
		return totalNum
	}
	number := totalVGPU.(resourceNumber)
	return int(number)
}

func (p *VGPUSchedulerPlugin) isVGPUResourcePod(state *framework.CycleState, pod *v1.Pod) bool {
	return p.getTotalRequestVGPUByPod(state, pod) > 0
}
