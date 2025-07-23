package plugin

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/coldzerofear/vgpu-manager/pkg/client"
	"github.com/coldzerofear/vgpu-manager/pkg/util"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

var _ framework.BindPlugin = &VGPUSchedulerPlugin{}

func (p *VGPUSchedulerPlugin) Lock() {
	p.mu.Lock()
	p.timestamp = time.Now().UnixMicro()
}

func (p *VGPUSchedulerPlugin) Unlock() {
	since := time.Since(time.UnixMicro(p.timestamp))
	klog.V(5).Infof("binding node took %d milliseconds", since.Milliseconds())
	if sleepTimestamp := 30*time.Millisecond - since; sleepTimestamp > 0 {
		time.Sleep(sleepTimestamp)
	}
	p.mu.Unlock()
}

func (p *VGPUSchedulerPlugin) Bind(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) *framework.Status {
	// Locking is to prevent excessive binding speed from causing device plugin allocation failed
	p.Lock()
	defer p.Unlock()

	logger := klog.FromContext(ctx)
	patchData := client.PatchMetadata{
		Annotations: map[string]string{},
		Labels:      map[string]string{},
	}
	if !p.isVGPUResourcePod(state, pod) {
		patchData.Labels[util.PodAssignedPhaseLabel] = string(util.AssignPhaseSucceed)
		patchData.Annotations[util.PodPredicateTimeAnnotation] = fmt.Sprintf("%d", uint64(math.MaxUint64))
	} else {
		data, err := state.Read(p.preAllocateDeviceKey(nodeName))
		if err != nil {
			errMsg := "getting pre allocated devices for node failed"
			logger.Error(err, errMsg, "pod", klog.KObj(pod), "node", nodeName)
			return framework.NewStatus(framework.Error, errMsg)
		}
		preAllocate := data.(preAllocateDevice)
		predicateTime := fmt.Sprintf("%d", metav1.NowMicro().UnixNano())
		patchData.Labels[util.PodAssignedPhaseLabel] = string(util.AssignPhaseSucceed)
		patchData.Annotations[util.PodPredicateNodeAnnotation] = nodeName
		patchData.Annotations[util.PodVGPUPreAllocAnnotation] = string(preAllocate)
		patchData.Annotations[util.PodPredicateTimeAnnotation] = predicateTime
	}

	err := retry.OnError(retry.DefaultRetry, util.ShouldRetry, func() error {
		return client.PatchPodMetadata(p.handle.ClientSet(), pod, patchData)
	})
	if err != nil {
		logger.Error(err, "patch vGPU metadata failed", "pod", klog.KObj(pod), "node", nodeName)
		return framework.NewStatus(framework.Error, err.Error())
	}

	binding := &v1.Binding{
		ObjectMeta: metav1.ObjectMeta{Name: pod.Name, Namespace: pod.Namespace, UID: pod.UID},
		Target:     v1.ObjectReference{Kind: "Node", Name: nodeName},
	}
	err = p.handle.ClientSet().CoreV1().Pods(pod.Namespace).Bind(ctx, binding, metav1.CreateOptions{})
	if err != nil {
		logger.Error(err, "Failed to bind pod to node", "pod", klog.KObj(pod), "node", nodeName)
		_ = client.PatchPodAllocationFailed(p.handle.ClientSet(), pod)
		return framework.NewStatus(framework.Error, err.Error())
	}
	logger.Info("Successfully bound pod to node", "pod", klog.KObj(pod), "node", nodeName)
	return framework.NewStatus(framework.Success, "")
}
