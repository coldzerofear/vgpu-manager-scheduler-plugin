package plugin

import (
	"context"

	"github.com/coldzerofear/vgpu-manager/pkg/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

var _ = Describe("VGPUSchedulerPlugin Filter", func() {
	var (
		plugin    *VGPUSchedulerPlugin
		ctx       context.Context
		testPod   *v1.Pod
		testState *framework.CycleState
		nodeInfo  *framework.NodeInfo
	)

	BeforeEach(func() {
		ctx = context.Background()
		testPod = &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
				UID:       uuid.NewUUID(),
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{{
					Name: "default",
					Resources: v1.ResourceRequirements{
						Limits:   v1.ResourceList{},
						Requests: v1.ResourceList{},
					},
				}},
			},
		}
		testState = framework.NewCycleState()
		plugin = &VGPUSchedulerPlugin{}
		nodeInfo = &framework.NodeInfo{
			Pods: []*framework.PodInfo{{
				Pod: &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod1",
						Namespace: "default",
						UID:       uuid.NewUUID(),
					},
					Spec: v1.PodSpec{
						Containers: []v1.Container{{
							Name: "default",
							Resources: v1.ResourceRequirements{
								Limits: v1.ResourceList{
									v1.ResourceCPU:    resource.MustParse("1"),
									v1.ResourceMemory: resource.MustParse("1Gi"),
								}},
						}},
					},
				},
			}, {
				Pod: &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod2",
						Namespace: "default",
						UID:       uuid.NewUUID(),
					},
					Spec: v1.PodSpec{
						Containers: []v1.Container{{
							Name: "default",
							Resources: v1.ResourceRequirements{
								Limits: v1.ResourceList{
									v1.ResourceCPU:    resource.MustParse("1"),
									v1.ResourceMemory: resource.MustParse("1Gi"),
								}},
						}},
					},
				},
			}},
		}
		nodeInfo.SetNode(&v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-pod2",
				UID:  uuid.NewUUID(),
			},
			Spec: v1.NodeSpec{},
			Status: v1.NodeStatus{
				Capacity:    map[v1.ResourceName]resource.Quantity{},
				Allocatable: map[v1.ResourceName]resource.Quantity{},
			},
		})
	})

	Context("When it's not a GPU node", func() {
		It("should return Unschedulable", func() {
			status := plugin.Filter(ctx, testState, testPod, nodeInfo)
			Expect(status.Code()).To(Equal(framework.Unschedulable))
			Expect(status.Message()).To(ContainSubstring("node does not have GPU"))
		})
	})

	Context("When the GPU of the node is insufficient", func() {
		BeforeEach(func() {
			testPod.Spec.Containers[0].Resources.Limits[util.VGPUNumberResourceName] = resource.MustParse("3")
			nodeInfo.Node().Status.Allocatable[util.VGPUNumberResourceName] = resource.MustParse("2")
		})
		It("should return Unschedulable", func() {
			status := plugin.Filter(ctx, testState, testPod, nodeInfo)
			Expect(status.Code()).To(Equal(framework.Unschedulable))
			Expect(status.Message()).To(ContainSubstring("insufficient GPU on the node"))
		})
	})
})
