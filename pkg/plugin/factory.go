package plugin

import (
	"context"
	"sync"

	"github.com/coldzerofear/vgpu-manager/cmd/scheduler/options"
	"k8s.io/apimachinery/pkg/runtime"
	v1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/component-base/featuregate"
	baseversion "k8s.io/component-base/version"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

const Name = "VGPUSchedulerPlugin"

func New(ctx context.Context, obj runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	featureGate := featuregate.NewFeatureGate()
	err := featureGate.Add(map[featuregate.Feature]featuregate.FeatureSpec{
		options.GPUTopology: {Default: true, PreRelease: featuregate.Alpha},
	})
	if err != nil {
		return nil, err
	}
	err = featuregate.DefaultComponentGlobalsRegistry.Register(
		options.Component, baseversion.DefaultBuildEffectiveVersion(), featureGate)
	if err != nil {
		return nil, err
	}
	podLister := handle.SharedInformerFactory().Core().V1().Pods().Lister()
	return &VGPUSchedulerPlugin{
		handle:    handle,
		podlister: podLister,
	}, nil
}

type VGPUSchedulerPlugin struct {
	mu        sync.Mutex
	timestamp int64
	handle    framework.Handle
	podlister v1.PodLister
}

func (p *VGPUSchedulerPlugin) Name() string {
	return Name
}

type resourceNumber int

func (r resourceNumber) Clone() framework.StateData {
	return r
}

type preAllocateDevice string

func (r preAllocateDevice) Clone() framework.StateData {
	return r
}

func (p *VGPUSchedulerPlugin) preAllocateDeviceKey(nodeName string) framework.StateKey {
	return framework.StateKey("PreAllocate_" + nodeName)
}
