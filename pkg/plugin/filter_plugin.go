package plugin

import (
	"context"
	"fmt"

	"github.com/coldzerofear/vgpu-manager/pkg/device"
	"github.com/coldzerofear/vgpu-manager/pkg/device/allocator"
	"github.com/coldzerofear/vgpu-manager/pkg/scheduler/filter"
	"github.com/coldzerofear/vgpu-manager/pkg/util"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

var _ framework.FilterPlugin = &VGPUSchedulerPlugin{}

func (p *VGPUSchedulerPlugin) Filter(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) (status *framework.Status) {
	defer func() {
		if !status.IsSuccess() {
			p.deleteDevNodeInfo(state, nodeInfo)
			state.Delete(p.preAllocateDeviceKey(nodeInfo.GetName()))
		}
	}()
	logger := klog.FromContext(ctx)
	// GPUs not provided on node, Unschedulable
	nodeVGPUNumber := util.GetAllocatableOfNode(nodeInfo.Node(), util.VGPUNumberResourceName)
	if nodeVGPUNumber == 0 {
		logger.Info("node does not have GPU", "node", nodeInfo.GetName())
		return framework.NewStatus(framework.Unschedulable, "node does not have GPU")
	}
	// The requested GPU exceeds the number provided by the node, Unschedulable
	if p.getTotalRequestVGPUByPod(state, pod) > nodeVGPUNumber {
		logger.Info("insufficient GPU on the node", "node", nodeInfo.GetName())
		return framework.NewStatus(framework.Unschedulable, "insufficient GPU on the node")
	}
	if status = p.nodeFilter(pod, nodeInfo); !status.IsSuccess() {
		logger.Error(fmt.Errorf("%s", status.String()), "node filter failed", "node", nodeInfo.GetName())
		return status
	}
	if status = p.deviceFilter(state, pod, nodeInfo); !status.IsSuccess() {
		logger.Error(fmt.Errorf("%s", status.String()), "device filter failed", "node", nodeInfo.GetName())
		return status
	}

	return framework.NewStatus(framework.Success, "")
}

func (p *VGPUSchedulerPlugin) deleteDevNodeInfo(state *framework.CycleState, nodeInfo *framework.NodeInfo) {
	state.Delete(framework.StateKey("DeviceNodeInfo_" + nodeInfo.GetName()))
}

func (p *VGPUSchedulerPlugin) createDevNodeInfo(state *framework.CycleState, nodeInfo *framework.NodeInfo) (*device.NodeInfo, error) {
	pods, err := p.podlister.List(labels.Everything())
	if err != nil {
		return nil, err
	}
	devNodeInfo, err := device.NewNodeInfo(nodeInfo.Node(), pods)
	if err != nil {
		return nil, err
	}
	nodeInfoKey := framework.StateKey("DeviceNodeInfo_" + nodeInfo.GetName())
	state.Write(nodeInfoKey, framework.StateData(devNodeInfo))
	return devNodeInfo, nil
}

func (p *VGPUSchedulerPlugin) getDevNodeInfo(state *framework.CycleState, nodeInfo *framework.NodeInfo) (*device.NodeInfo, error) {
	var devNodeInfo *device.NodeInfo
	nodeInfoKey := framework.StateKey("DeviceNodeInfo_" + nodeInfo.GetName())
	if data, err := state.Read(nodeInfoKey); err != nil {
		devNodeInfo, err = p.createDevNodeInfo(state, nodeInfo)
		if err != nil {
			return nil, err
		}
	} else {
		devNodeInfo = data.(*device.NodeInfo)
	}
	return devNodeInfo, nil
}

func (p *VGPUSchedulerPlugin) deviceFilter(state *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) (status *framework.Status) {
	devNodeInfo, err := p.createDevNodeInfo(state, nodeInfo)
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
	state.Write(p.preAllocateDeviceKey(nodeInfo.GetName()), allocateDevice)
	return framework.NewStatus(framework.Success, "")
}

func (p *VGPUSchedulerPlugin) nodeFilter(pod *v1.Pod, nodeInfo *framework.NodeInfo) (status *framework.Status) {
	memoryPolicyFunc := filter.GetMemoryPolicyFunc(pod)
	if err := filter.CheckNode(nodeInfo.Node(), memoryPolicyFunc); err != nil {
		return framework.NewStatus(framework.Unschedulable, err.Error())
	}
	return framework.NewStatus(framework.Success, "")
}
