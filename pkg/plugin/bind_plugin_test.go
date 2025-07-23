package plugin

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/coldzerofear/vgpu-manager/pkg/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	testing2 "k8s.io/client-go/testing"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

func Test_VGPUSchedulerPlugin(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VGPUSchedulerPlugin Test Suite")
}

var _ = Describe("VGPUSchedulerPlugin Bind", func() {
	var (
		plugin    *VGPUSchedulerPlugin
		ctx       context.Context
		fakeCli   *fake.Clientset
		testPod   *v1.Pod
		testState *framework.CycleState
		nodeName  = "test-node"
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeCli = fake.NewSimpleClientset()
		testPod = &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
				UID:       uuid.NewUUID(),
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{{
					Name: "default",
				}},
			},
		}
		testState = framework.NewCycleState()
		plugin = &VGPUSchedulerPlugin{
			handle: &frameworkHandleStub{
				clientSet: fakeCli,
			},
		}
	})

	Context("when binding non-vGPU pod", func() {
		BeforeEach(func() {
			testPod.Spec.Containers[0].Resources = v1.ResourceRequirements{
				Limits: v1.ResourceList{
					v1.ResourceCPU:    resource.MustParse("1"),
					v1.ResourceMemory: resource.MustParse("1Gi"),
				},
			}
			_, _ = fakeCli.CoreV1().Pods(testPod.Namespace).Create(ctx, testPod, metav1.CreateOptions{})
		})
		It("should succeed with correct labels and annotations", func() {

			status := plugin.Bind(ctx, testState, testPod, nodeName)
			Expect(status.IsSuccess()).To(BeTrue())
			updatedPod, err := fakeCli.CoreV1().Pods(testPod.Namespace).Get(ctx, testPod.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			//Expect(updatedPod.Spec.NodeName).To(Equal(nodeName))
			Expect(updatedPod.Labels[util.PodAssignedPhaseLabel]).To(Equal(string(util.AssignPhaseSucceed)))
			Expect(updatedPod.Annotations[util.PodPredicateTimeAnnotation]).To(Equal(fmt.Sprintf("%d", uint64(math.MaxUint64))))
		})
		AfterEach(func() {
			_ = fakeCli.CoreV1().Pods(testPod.Namespace).Delete(ctx, testPod.Name, metav1.DeleteOptions{})
		})
	})

	Context("when binding vGPU pod", func() {
		preAllocStr := "default[0_GPU-xxxx_0_2048]"
		BeforeEach(func() {
			preAlloc := preAllocateDevice(preAllocStr)
			testState.Write(plugin.preAllocateDeviceKey(nodeName), preAlloc)
			testPod.Spec.Containers[0].Resources = v1.ResourceRequirements{
				Limits: v1.ResourceList{
					v1.ResourceCPU:              resource.MustParse("1"),
					v1.ResourceMemory:           resource.MustParse("1Gi"),
					util.VGPUNumberResourceName: resource.MustParse("1"),
				},
			}
			_, _ = fakeCli.CoreV1().Pods(testPod.Namespace).Create(ctx, testPod, metav1.CreateOptions{})
		})
		It("should succeed with vGPU annotations", func() {
			status := plugin.Bind(ctx, testState, testPod, nodeName)
			Expect(status.IsSuccess()).To(BeTrue())

			updatedPod, err := fakeCli.CoreV1().Pods(testPod.Namespace).Get(ctx, testPod.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			//Expect(updatedPod.Spec.NodeName).To(Equal(nodeName))
			Expect(updatedPod.Labels[util.PodAssignedPhaseLabel]).To(Equal(string(util.AssignPhaseAllocating)))
			Expect(updatedPod.Annotations[util.PodVGPUPreAllocAnnotation]).To(Equal(preAllocStr))
			Expect(updatedPod.Annotations[util.PodPredicateNodeAnnotation]).To(Equal(nodeName))
			Expect(updatedPod.Annotations[util.PodVGPURealAllocAnnotation]).To(Equal(""))
		})
		AfterEach(func() {
			testState.Delete(plugin.preAllocateDeviceKey(nodeName))
			_ = fakeCli.CoreV1().Pods(testPod.Namespace).Delete(ctx, testPod.Name, metav1.DeleteOptions{})
		})
	})

	Context("when state read fails", func() {
		BeforeEach(func() {
			testPod.Spec.Containers[0].Resources = v1.ResourceRequirements{
				Limits: v1.ResourceList{
					v1.ResourceCPU:              resource.MustParse("1"),
					v1.ResourceMemory:           resource.MustParse("1Gi"),
					util.VGPUNumberResourceName: resource.MustParse("1"),
				},
			}
			testState.Delete(plugin.preAllocateDeviceKey(nodeName))
		})
		It("should return error status", func() {
			status := plugin.Bind(ctx, testState, testPod, nodeName)
			Expect(status.Code()).To(Equal(framework.Error))
			Expect(status.Message()).To(ContainSubstring("getting pre allocated devices for node failed"))
		})
	})

	Context("when patch metadata fails", func() {
		patchErr := errors.New("patch error")
		BeforeEach(func() {
			preAllocStr := "default[0_GPU-xxxx_0_2048]"
			preAlloc := preAllocateDevice(preAllocStr)
			testState.Write(plugin.preAllocateDeviceKey(nodeName), preAlloc)
			fakeCli.PrependReactor("patch", "pods", func(action testing2.Action) (handled bool, ret runtime.Object, err error) {
				return true, nil, patchErr
			})
		})

		It("should return error status", func() {
			status := plugin.Bind(ctx, testState, testPod, nodeName)
			Expect(status.Code()).To(Equal(framework.Error))
			Expect(status.Message()).To(ContainSubstring(patchErr.Error()))
		})

		AfterEach(func() {
			testState.Delete(plugin.preAllocateDeviceKey(nodeName))
		})
	})

	Context("when bind operation fails", func() {
		preAllocStr := "default[0_GPU-xxxx_0_2048]"
		BeforeEach(func() {
			preAlloc := preAllocateDevice(preAllocStr)
			testState.Write(plugin.preAllocateDeviceKey(nodeName), preAlloc)
			fakeCli.PrependReactor("create", "pods", func(action testing2.Action) (bool, runtime.Object, error) {
				if action.GetSubresource() == "binding" {
					return true, nil, errors.New("bind failed")
				}
				return false, nil, nil
			})
			testPod.Spec.Containers[0].Resources = v1.ResourceRequirements{
				Limits: v1.ResourceList{
					v1.ResourceCPU:              resource.MustParse("1"),
					v1.ResourceMemory:           resource.MustParse("1Gi"),
					util.VGPUNumberResourceName: resource.MustParse("1"),
				},
			}
			_, _ = fakeCli.CoreV1().Pods(testPod.Namespace).Create(ctx, testPod, metav1.CreateOptions{})
		})

		It("should return error status and mark allocation failed", func() {
			status := plugin.Bind(ctx, testState, testPod, nodeName)
			Expect(status.Code()).To(Equal(framework.Error))
			updatedPod, err := fakeCli.CoreV1().Pods(testPod.Namespace).Get(ctx, testPod.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedPod.Spec.NodeName).To(BeEmpty())
			Expect(updatedPod.Labels[util.PodAssignedPhaseLabel]).To(Equal(string(util.AssignPhaseFailed)))
			Expect(updatedPod.Annotations[util.PodPredicateTimeAnnotation]).To(Equal(fmt.Sprintf("%d", uint64(math.MaxUint64))))
		})
		AfterEach(func() {
			testState.Delete(plugin.preAllocateDeviceKey(nodeName))
			_ = fakeCli.CoreV1().Pods(testPod.Namespace).Delete(ctx, testPod.Name, metav1.DeleteOptions{})
		})
	})
})

type frameworkHandleStub struct {
	framework.Handle
	clientSet *fake.Clientset
}

func (h *frameworkHandleStub) ClientSet() clientset.Interface {
	return h.clientSet
}
