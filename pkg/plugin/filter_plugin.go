package plugin

import (
	"context"
	"fmt"

	"github.com/coldzerofear/vgpu-manager/pkg/device"
	"github.com/coldzerofear/vgpu-manager/pkg/device/allocator"
	"github.com/coldzerofear/vgpu-manager/pkg/scheduler/filter"
	"github.com/coldzerofear/vgpu-manager/pkg/util"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

var _ framework.FilterPlugin = &VGPUSchedulerPlugin{}

func (p *VGPUSchedulerPlugin) Filter(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) (status *framework.Status) {
	logger := klog.FromContext(ctx)
	//defer func() {
	//	if r := recover(); r != nil {
	//		err := fmt.Errorf("internal error! recovered from panic: %v", r)
	//		logger.Error(err, "")
	//		status = framework.NewStatus(framework.Error, err.Error())
	//	}
	//}()
	// GPUs not provided on node, Unschedulable
	nodeVGPUNumber := util.GetAllocatableOfNode(nodeInfo.Node(), util.VGPUNumberResourceName)
	if nodeVGPUNumber == 0 {
		logger.Info("node does not have GPU", "node", nodeInfo.Node().Name)
		return framework.NewStatus(framework.Unschedulable, "node does not have GPU")
	}
	// The requested GPU exceeds the number provided by the node, Unschedulable
	if p.getTotalRequestVGPUByPod(state, pod) > nodeVGPUNumber {
		logger.Info("insufficient GPU on the node", "node", nodeInfo.Node().Name)
		return framework.NewStatus(framework.Unschedulable, "insufficient GPU on the node")
	}
	if result := p.nodeFilter(pod, nodeInfo); !result.IsSuccess() {
		logger.Error(fmt.Errorf("%s", result.String()), "node filter failed", "node", nodeInfo.Node().Name)
		return result
	}
	if result := p.deviceFilter(state, pod, nodeInfo); !result.IsSuccess() {
		logger.Error(fmt.Errorf("%s", result.String()), "device filter failed", "node", nodeInfo.Node().Name)
		return result
	}

	return framework.NewStatus(framework.Success, "")
}

func (p *VGPUSchedulerPlugin) getDevNodeInfo(state *framework.CycleState, nodeInfo *framework.NodeInfo) (*device.NodeInfo, error) {
	nodeInfoKey := framework.StateKey("DeviceNodeInfo_" + nodeInfo.GetName())
	data, err := state.Read(nodeInfoKey)
	if err != nil {
		devNodeInfo, err := device.NewNodeInfoByNodeInfo(nodeInfo)
		if err != nil {
			return nil, err
		}
		data = framework.StateData(devNodeInfo)
		state.Write(nodeInfoKey, data)
		return devNodeInfo, nil
	}
	devNodeInfo := data.(*device.NodeInfo)
	return devNodeInfo, nil
}

func (p *VGPUSchedulerPlugin) deviceFilter(state *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) (status *framework.Status) {
	devNodeInfo, err := p.getDevNodeInfo(state, nodeInfo)
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}
	devNodeInfo = devNodeInfo.Clone().(*device.NodeInfo)
	newPod, err := allocator.NewAllocator(devNodeInfo).Allocate(pod)
	if err != nil {
		return framework.NewStatus(framework.Unschedulable, err.Error())
	}
	preAllocate := newPod.Annotations[util.PodVGPUPreAllocAnnotation]
	allocateDevice := preAllocateDevice(preAllocate)
	state.Write(p.preAllocateDeviceKey(nodeInfo.Node().Name), allocateDevice)
	return framework.NewStatus(framework.Success, "")
}

func (p *VGPUSchedulerPlugin) nodeFilter(pod *v1.Pod, nodeInfo *framework.NodeInfo) (status *framework.Status) {
	memoryPolicyFunc := filter.GetMemoryPolicyFunc(pod)
	if err := filter.CheckNode(nodeInfo.Node(), memoryPolicyFunc); err != nil {
		return framework.NewStatus(framework.Unschedulable, err.Error())
	}
	return framework.NewStatus(framework.Success, "")
}
