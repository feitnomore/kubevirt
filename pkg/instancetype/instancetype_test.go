//nolint:dupl,lll
package instancetype_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/golang/mock/gomock"
	appsv1 "k8s.io/api/apps/v1"
	k8sv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	k8sfield "k8s.io/apimachinery/pkg/util/validation/field"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"kubevirt.io/client-go/api"
	"kubevirt.io/client-go/kubecli"

	"kubevirt.io/kubevirt/pkg/instancetype"
	"kubevirt.io/kubevirt/pkg/instancetype/conflict"
	"kubevirt.io/kubevirt/pkg/instancetype/preference/requirements"
	"kubevirt.io/kubevirt/pkg/pointer"
	"kubevirt.io/kubevirt/pkg/testutils"

	v1 "kubevirt.io/api/core/v1"
	apiinstancetype "kubevirt.io/api/instancetype"
	instancetypev1beta1 "kubevirt.io/api/instancetype/v1beta1"
)

var _ = Describe("Instancetype and Preferences", func() {
	var (
		ctrl                             *gomock.Controller
		instancetypeMethods              instancetype.Methods
		vm                               *v1.VirtualMachine
		vmi                              *v1.VirtualMachineInstance
		virtClient                       *kubecli.MockKubevirtClient
		vmInterface                      *kubecli.MockVirtualMachineInterface
		k8sClient                        *k8sfake.Clientset
		instancetypeInformerStore        cache.Store
		clusterInstancetypeInformerStore cache.Store
		preferenceInformerStore          cache.Store
		clusterPreferenceInformerStore   cache.Store
		controllerrevisionInformerStore  cache.Store
	)

	BeforeEach(func() {
		k8sClient = k8sfake.NewSimpleClientset()
		ctrl = gomock.NewController(GinkgoT())
		virtClient = kubecli.NewMockKubevirtClient(ctrl)
		vmInterface = kubecli.NewMockVirtualMachineInterface(ctrl)
		virtClient.EXPECT().VirtualMachine(metav1.NamespaceDefault).Return(vmInterface).AnyTimes()
		virtClient.EXPECT().AppsV1().Return(k8sClient.AppsV1()).AnyTimes()

		instancetypeInformer, _ := testutils.NewFakeInformerFor(&instancetypev1beta1.VirtualMachineInstancetype{})
		instancetypeInformerStore = instancetypeInformer.GetStore()

		clusterInstancetypeInformer, _ := testutils.NewFakeInformerFor(&instancetypev1beta1.VirtualMachineClusterInstancetype{})
		clusterInstancetypeInformerStore = clusterInstancetypeInformer.GetStore()

		preferenceInformer, _ := testutils.NewFakeInformerFor(&instancetypev1beta1.VirtualMachinePreference{})
		preferenceInformerStore = preferenceInformer.GetStore()

		clusterPreferenceInformer, _ := testutils.NewFakeInformerFor(&instancetypev1beta1.VirtualMachineClusterPreference{})
		clusterPreferenceInformerStore = clusterPreferenceInformer.GetStore()

		controllerrevisionInformer, _ := testutils.NewFakeInformerFor(&appsv1.ControllerRevision{})
		controllerrevisionInformerStore = controllerrevisionInformer.GetStore()

		instancetypeMethods = &instancetype.InstancetypeMethods{
			InstancetypeStore:        instancetypeInformerStore,
			ClusterInstancetypeStore: clusterInstancetypeInformerStore,
			PreferenceStore:          preferenceInformerStore,
			ClusterPreferenceStore:   clusterPreferenceInformerStore,
			ControllerRevisionStore:  controllerrevisionInformerStore,
			Clientset:                virtClient,
		}

		vm = kubecli.NewMinimalVM("testvm")
		vm.Spec.Template = &v1.VirtualMachineInstanceTemplateSpec{
			Spec: v1.VirtualMachineInstanceSpec{
				Domain: v1.DomainSpec{},
			},
		}
		vm.Namespace = k8sv1.NamespaceDefault
	})

	Context("Add instancetype name annotations", func() {
		const instancetypeName = "instancetype-name"

		BeforeEach(func() {
			vm = kubecli.NewMinimalVM("testvm")
			vm.Spec.Instancetype = &v1.InstancetypeMatcher{Name: instancetypeName}
		})

		It("should add instancetype name annotation", func() {
			vm.Spec.Instancetype.Kind = apiinstancetype.SingularResourceName

			meta := &metav1.ObjectMeta{}
			instancetype.AddInstancetypeNameAnnotations(vm, meta)

			Expect(meta.Annotations[v1.InstancetypeAnnotation]).To(Equal(instancetypeName))
			Expect(meta.Annotations[v1.ClusterInstancetypeAnnotation]).To(Equal(""))
		})

		It("should add cluster instancetype name annotation", func() {
			vm.Spec.Instancetype.Kind = apiinstancetype.ClusterSingularResourceName

			meta := &metav1.ObjectMeta{}
			instancetype.AddInstancetypeNameAnnotations(vm, meta)

			Expect(meta.Annotations[v1.InstancetypeAnnotation]).To(Equal(""))
			Expect(meta.Annotations[v1.ClusterInstancetypeAnnotation]).To(Equal(instancetypeName))
		})

		It("should add cluster name annotation, if instancetype.kind is empty", func() {
			vm.Spec.Instancetype.Kind = ""

			meta := &metav1.ObjectMeta{}
			instancetype.AddInstancetypeNameAnnotations(vm, meta)

			Expect(meta.Annotations[v1.InstancetypeAnnotation]).To(Equal(""))
			Expect(meta.Annotations[v1.ClusterInstancetypeAnnotation]).To(Equal(instancetypeName))
		})
	})

	Context("Add preference name annotations", func() {
		const preferenceName = "preference-name"

		BeforeEach(func() {
			vm = kubecli.NewMinimalVM("testvm")
			vm.Spec.Preference = &v1.PreferenceMatcher{Name: preferenceName}
		})

		It("should add preference name annotation", func() {
			vm.Spec.Preference.Kind = apiinstancetype.SingularPreferenceResourceName

			meta := &metav1.ObjectMeta{}
			instancetype.AddPreferenceNameAnnotations(vm, meta)

			Expect(meta.Annotations[v1.PreferenceAnnotation]).To(Equal(preferenceName))
			Expect(meta.Annotations[v1.ClusterPreferenceAnnotation]).To(Equal(""))
		})

		It("should add cluster preference name annotation", func() {
			vm.Spec.Preference.Kind = apiinstancetype.ClusterSingularPreferenceResourceName

			meta := &metav1.ObjectMeta{}
			instancetype.AddPreferenceNameAnnotations(vm, meta)

			Expect(meta.Annotations[v1.PreferenceAnnotation]).To(Equal(""))
			Expect(meta.Annotations[v1.ClusterPreferenceAnnotation]).To(Equal(preferenceName))
		})

		It("should add cluster name annotation, if preference.kind is empty", func() {
			vm.Spec.Preference.Kind = ""

			meta := &metav1.ObjectMeta{}
			instancetype.AddPreferenceNameAnnotations(vm, meta)

			Expect(meta.Annotations[v1.PreferenceAnnotation]).To(Equal(""))
			Expect(meta.Annotations[v1.ClusterPreferenceAnnotation]).To(Equal(preferenceName))
		})
	})

	Context("Apply", func() {
		var (
			instancetypeSpec *instancetypev1beta1.VirtualMachineInstancetypeSpec
			preferenceSpec   *instancetypev1beta1.VirtualMachinePreferenceSpec
			field            *k8sfield.Path
		)

		BeforeEach(func() {
			vmi = api.NewMinimalVMI("testvmi")

			vmi.Spec = v1.VirtualMachineInstanceSpec{
				Domain: v1.DomainSpec{},
			}
			field = k8sfield.NewPath("spec", "template", "spec")
		})

		Context("instancetype.spec.NodeSelector", func() {
			It("should apply to VMI", func() {
				instancetypeSpec = &instancetypev1beta1.VirtualMachineInstancetypeSpec{
					NodeSelector: map[string]string{"key": "value"},
				}

				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(BeEmpty())
				Expect(vmi.Spec.NodeSelector).To(Equal(instancetypeSpec.NodeSelector))
			})

			It("should be no-op if vmi.Spec.NodeSelector is already set but instancetype.NodeSelector is empty", func() {
				instancetypeSpec = &instancetypev1beta1.VirtualMachineInstancetypeSpec{}
				vmi.Spec.NodeSelector = map[string]string{"key": "value"}

				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(BeEmpty())
				Expect(vmi.Spec.NodeSelector).To(Equal(map[string]string{"key": "value"}))
			})

			It("should return a conflict if vmi.Spec.NodeSelector is already set and instancetype.NodeSelector is defined", func() {
				instancetypeSpec = &instancetypev1beta1.VirtualMachineInstancetypeSpec{
					NodeSelector: map[string]string{"key": "value"},
				}
				vmi.Spec.NodeSelector = map[string]string{"key": "value"}

				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(HaveLen(1))
				Expect(conflicts[0].String()).To(Equal("spec.template.spec.nodeSelector"))
			})
		})

		Context("instancetype.spec.SchedulerName", func() {
			It("should apply to VMI", func() {
				instancetypeSpec = &instancetypev1beta1.VirtualMachineInstancetypeSpec{
					SchedulerName: "ultra-scheduler",
				}

				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(BeEmpty())
				Expect(vmi.Spec.SchedulerName).To(Equal(instancetypeSpec.SchedulerName))
			})

			It("should be no-op if vmi.Spec.SchedulerName is already set but instancetype.SchedulerName is empty", func() {
				instancetypeSpec = &instancetypev1beta1.VirtualMachineInstancetypeSpec{}
				vmi.Spec.SchedulerName = "super-fast-scheduler"

				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(BeEmpty())
				Expect(vmi.Spec.SchedulerName).To(Equal("super-fast-scheduler"))
			})

			It("should return a conflict if vmi.Spec.SchedulerName is already set and instancetype.SchedulerName is defined", func() {
				instancetypeSpec = &instancetypev1beta1.VirtualMachineInstancetypeSpec{
					SchedulerName: "ultra-fast-scheduler",
				}
				vmi.Spec.SchedulerName = "slow-scheduler"

				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(HaveLen(1))
				Expect(conflicts[0].String()).To(Equal("spec.template.spec.schedulerName"))
			})
		})

		Context("instancetype.spec.CPU and preference.spec.CPU", func() {
			BeforeEach(func() {
				instancetypeSpec = &instancetypev1beta1.VirtualMachineInstancetypeSpec{
					CPU: instancetypev1beta1.CPUInstancetype{
						Guest:                 uint32(2),
						Model:                 pointer.P("host-passthrough"),
						DedicatedCPUPlacement: pointer.P(true),
						IsolateEmulatorThread: pointer.P(true),
						NUMA: &v1.NUMA{
							GuestMappingPassthrough: &v1.NUMAGuestMappingPassthrough{},
						},
						Realtime: &v1.Realtime{
							Mask: "0-3,^1",
						},
						MaxSockets: pointer.P(uint32(6)),
					},
				}
				preferenceSpec = &instancetypev1beta1.VirtualMachinePreferenceSpec{
					CPU: &instancetypev1beta1.CPUPreferences{},
				}
			})

			It("should default to PreferSockets", func() {
				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(BeEmpty())

				Expect(vmi.Spec.Domain.CPU.Sockets).To(Equal(instancetypeSpec.CPU.Guest))
				Expect(vmi.Spec.Domain.CPU.Cores).To(Equal(uint32(1)))
				Expect(vmi.Spec.Domain.CPU.Threads).To(Equal(uint32(1)))
				Expect(vmi.Spec.Domain.CPU.Model).To(Equal(*instancetypeSpec.CPU.Model))
				Expect(vmi.Spec.Domain.CPU.DedicatedCPUPlacement).To(Equal(*instancetypeSpec.CPU.DedicatedCPUPlacement))
				Expect(vmi.Spec.Domain.CPU.IsolateEmulatorThread).To(Equal(*instancetypeSpec.CPU.IsolateEmulatorThread))
				Expect(*vmi.Spec.Domain.CPU.NUMA).To(Equal(*instancetypeSpec.CPU.NUMA))
				Expect(*vmi.Spec.Domain.CPU.Realtime).To(Equal(*instancetypeSpec.CPU.Realtime))
				Expect(vmi.Spec.Domain.CPU.MaxSockets).To(Equal(*instancetypeSpec.CPU.MaxSockets))
			})

			It("should default to Sockets, when instancetype is used with PreferAny", func() {
				preferredCPUTopology := instancetypev1beta1.Any
				preferenceSpec.CPU.PreferredCPUTopology = &preferredCPUTopology

				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(BeEmpty())
				Expect(vmi.Spec.Domain.CPU.Sockets).To(Equal(instancetypeSpec.CPU.Guest))
				Expect(vmi.Spec.Domain.CPU.Cores).To(Equal(uint32(1)))
				Expect(vmi.Spec.Domain.CPU.Threads).To(Equal(uint32(1)))
			})

			It("should apply in full with PreferCores selected", func() {
				preferredCPUTopology := instancetypev1beta1.Cores
				preferenceSpec.CPU.PreferredCPUTopology = &preferredCPUTopology

				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(BeEmpty())

				Expect(vmi.Spec.Domain.CPU.Sockets).To(Equal(uint32(1)))
				Expect(vmi.Spec.Domain.CPU.Cores).To(Equal(instancetypeSpec.CPU.Guest))
				Expect(vmi.Spec.Domain.CPU.Threads).To(Equal(uint32(1)))
				Expect(vmi.Spec.Domain.CPU.Model).To(Equal(*instancetypeSpec.CPU.Model))
				Expect(vmi.Spec.Domain.CPU.DedicatedCPUPlacement).To(Equal(*instancetypeSpec.CPU.DedicatedCPUPlacement))
				Expect(vmi.Spec.Domain.CPU.IsolateEmulatorThread).To(Equal(*instancetypeSpec.CPU.IsolateEmulatorThread))
				Expect(*vmi.Spec.Domain.CPU.NUMA).To(Equal(*instancetypeSpec.CPU.NUMA))
				Expect(*vmi.Spec.Domain.CPU.Realtime).To(Equal(*instancetypeSpec.CPU.Realtime))
			})

			It("should apply in full with PreferThreads selected", func() {
				preferredCPUTopology := instancetypev1beta1.Threads
				preferenceSpec.CPU.PreferredCPUTopology = &preferredCPUTopology

				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(BeEmpty())

				Expect(vmi.Spec.Domain.CPU.Sockets).To(Equal(uint32(1)))
				Expect(vmi.Spec.Domain.CPU.Cores).To(Equal(uint32(1)))
				Expect(vmi.Spec.Domain.CPU.Threads).To(Equal(instancetypeSpec.CPU.Guest))
				Expect(vmi.Spec.Domain.CPU.Model).To(Equal(*instancetypeSpec.CPU.Model))
				Expect(vmi.Spec.Domain.CPU.DedicatedCPUPlacement).To(Equal(*instancetypeSpec.CPU.DedicatedCPUPlacement))
				Expect(vmi.Spec.Domain.CPU.IsolateEmulatorThread).To(Equal(*instancetypeSpec.CPU.IsolateEmulatorThread))
				Expect(*vmi.Spec.Domain.CPU.NUMA).To(Equal(*instancetypeSpec.CPU.NUMA))
				Expect(*vmi.Spec.Domain.CPU.Realtime).To(Equal(*instancetypeSpec.CPU.Realtime))
			})

			It("should apply in full with PreferSockets selected", func() {
				preferredCPUTopology := instancetypev1beta1.Sockets
				preferenceSpec.CPU.PreferredCPUTopology = &preferredCPUTopology

				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(BeEmpty())

				Expect(vmi.Spec.Domain.CPU.Sockets).To(Equal(instancetypeSpec.CPU.Guest))
				Expect(vmi.Spec.Domain.CPU.Cores).To(Equal(uint32(1)))
				Expect(vmi.Spec.Domain.CPU.Threads).To(Equal(uint32(1)))
				Expect(vmi.Spec.Domain.CPU.Model).To(Equal(*instancetypeSpec.CPU.Model))
				Expect(vmi.Spec.Domain.CPU.DedicatedCPUPlacement).To(Equal(*instancetypeSpec.CPU.DedicatedCPUPlacement))
				Expect(vmi.Spec.Domain.CPU.IsolateEmulatorThread).To(Equal(*instancetypeSpec.CPU.IsolateEmulatorThread))
				Expect(*vmi.Spec.Domain.CPU.NUMA).To(Equal(*instancetypeSpec.CPU.NUMA))
				Expect(*vmi.Spec.Domain.CPU.Realtime).To(Equal(*instancetypeSpec.CPU.Realtime))
			})

			Context("with PreferSpread", func() {
				DescribeTable("should spread", func(vCPUs uint32, preferenceSpec instancetypev1beta1.VirtualMachinePreferenceSpec, expectedCPU v1.CPU) {
					instancetypeSpec.CPU.Guest = vCPUs
					if preferenceSpec.CPU == nil {
						preferenceSpec.CPU = &instancetypev1beta1.CPUPreferences{}
					}
					preferenceSpec.CPU.PreferredCPUTopology = pointer.P(instancetypev1beta1.Spread)

					Expect(instancetypeMethods.ApplyToVmi(field, instancetypeSpec, &preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)).To(BeEmpty())
					Expect(vmi.Spec.Domain.CPU.Sockets).To(Equal(expectedCPU.Sockets))
					Expect(vmi.Spec.Domain.CPU.Cores).To(Equal(expectedCPU.Cores))
					Expect(vmi.Spec.Domain.CPU.Threads).To(Equal(expectedCPU.Threads))
				},
					Entry("by default to SocketsCores with a default topology for 1 vCPU",
						uint32(1),
						instancetypev1beta1.VirtualMachinePreferenceSpec{},
						v1.CPU{Sockets: 1, Cores: 1, Threads: 1},
					),
					Entry("by default to SocketsCores with 2 vCPUs and a default ratio of 1:2:1",
						uint32(2),
						instancetypev1beta1.VirtualMachinePreferenceSpec{},
						v1.CPU{Sockets: 1, Cores: 2, Threads: 1},
					),
					Entry("by default to SocketsCores with 4 vCPUs and a default ratio of 1:2:1",
						uint32(4),
						instancetypev1beta1.VirtualMachinePreferenceSpec{},
						v1.CPU{Sockets: 2, Cores: 2, Threads: 1},
					),
					Entry("by default to SocketsCores with 6 vCPUs and a default ratio of 1:2:1",
						uint32(6),
						instancetypev1beta1.VirtualMachinePreferenceSpec{},
						v1.CPU{Sockets: 3, Cores: 2, Threads: 1},
					),
					Entry("by default to SocketsCores with 8 vCPUs and a default ratio of 1:2:1",
						uint32(8),
						instancetypev1beta1.VirtualMachinePreferenceSpec{},
						v1.CPU{Sockets: 4, Cores: 2, Threads: 1},
					),
					Entry("by default to SocketsCores with 3 vCPUs and a ratio of 1:3:1",
						uint32(3),
						instancetypev1beta1.VirtualMachinePreferenceSpec{
							CPU: &instancetypev1beta1.CPUPreferences{
								SpreadOptions: &instancetypev1beta1.SpreadOptions{
									Ratio: pointer.P(uint32(3)),
								},
							},
						},
						v1.CPU{Sockets: 1, Cores: 3, Threads: 1},
					),
					Entry("by default to SocketsCores with 6 vCPUs and a ratio of 1:3:1",
						uint32(6),
						instancetypev1beta1.VirtualMachinePreferenceSpec{
							CPU: &instancetypev1beta1.CPUPreferences{
								SpreadOptions: &instancetypev1beta1.SpreadOptions{
									Ratio: pointer.P(uint32(3)),
								},
							},
						},
						v1.CPU{Sockets: 2, Cores: 3, Threads: 1},
					),
					Entry("by default to SocketsCores with 9 vCPUs and a ratio of 1:3:1",
						uint32(9),
						instancetypev1beta1.VirtualMachinePreferenceSpec{
							CPU: &instancetypev1beta1.CPUPreferences{
								SpreadOptions: &instancetypev1beta1.SpreadOptions{
									Ratio: pointer.P(uint32(3)),
								},
							},
						},
						v1.CPU{Sockets: 3, Cores: 3, Threads: 1},
					),
					Entry("by default to SocketsCores with 12 vCPUs and a ratio of 1:3:1",
						uint32(12),
						instancetypev1beta1.VirtualMachinePreferenceSpec{
							CPU: &instancetypev1beta1.CPUPreferences{
								SpreadOptions: &instancetypev1beta1.SpreadOptions{
									Ratio: pointer.P(uint32(3)),
								},
							},
						},
						v1.CPU{Sockets: 4, Cores: 3, Threads: 1},
					),
					Entry("by default to SocketsCores with 4 vCPUs and a ratio of 1:4:1",
						uint32(4),
						instancetypev1beta1.VirtualMachinePreferenceSpec{
							CPU: &instancetypev1beta1.CPUPreferences{
								SpreadOptions: &instancetypev1beta1.SpreadOptions{
									Ratio: pointer.P(uint32(4)),
								},
							},
						},
						v1.CPU{Sockets: 1, Cores: 4, Threads: 1},
					),
					Entry("by default to SocketsCores with 8 vCPUs and a ratio of 1:4:1",
						uint32(8),
						instancetypev1beta1.VirtualMachinePreferenceSpec{
							CPU: &instancetypev1beta1.CPUPreferences{
								SpreadOptions: &instancetypev1beta1.SpreadOptions{
									Ratio: pointer.P(uint32(4)),
								},
							},
						},
						v1.CPU{Sockets: 2, Cores: 4, Threads: 1},
					),
					Entry("by default to SocketsCores with 12 vCPUs and a ratio of 1:4:1",
						uint32(12),
						instancetypev1beta1.VirtualMachinePreferenceSpec{
							CPU: &instancetypev1beta1.CPUPreferences{
								SpreadOptions: &instancetypev1beta1.SpreadOptions{
									Ratio: pointer.P(uint32(4)),
								},
							},
						},
						v1.CPU{Sockets: 3, Cores: 4, Threads: 1},
					),
					Entry("by default to SocketsCores with 16 vCPUs and a ratio of 1:4:1",
						uint32(16),
						instancetypev1beta1.VirtualMachinePreferenceSpec{
							CPU: &instancetypev1beta1.CPUPreferences{
								SpreadOptions: &instancetypev1beta1.SpreadOptions{
									Ratio: pointer.P(uint32(4)),
								},
							},
						},
						v1.CPU{Sockets: 4, Cores: 4, Threads: 1},
					),
					Entry("to SocketsCoresThreads with 4 vCPUs and a default ratio of 1:2:2",
						uint32(4),
						instancetypev1beta1.VirtualMachinePreferenceSpec{
							CPU: &instancetypev1beta1.CPUPreferences{
								SpreadOptions: &instancetypev1beta1.SpreadOptions{
									Across: pointer.P(instancetypev1beta1.SpreadAcrossSocketsCoresThreads),
								},
							},
						},
						v1.CPU{Sockets: 1, Cores: 2, Threads: 2},
					),
					Entry("to SocketsCoresThreads with 8 vCPUs and a default ratio of 1:2:2",
						uint32(8),
						instancetypev1beta1.VirtualMachinePreferenceSpec{
							CPU: &instancetypev1beta1.CPUPreferences{
								SpreadOptions: &instancetypev1beta1.SpreadOptions{
									Across: pointer.P(instancetypev1beta1.SpreadAcrossSocketsCoresThreads),
								},
							},
						},
						v1.CPU{Sockets: 2, Cores: 2, Threads: 2},
					),
					Entry("to SocketsCoresThreads with 12 vCPUs and a default ratio of 1:2:2",
						uint32(12),
						instancetypev1beta1.VirtualMachinePreferenceSpec{
							CPU: &instancetypev1beta1.CPUPreferences{
								SpreadOptions: &instancetypev1beta1.SpreadOptions{
									Across: pointer.P(instancetypev1beta1.SpreadAcrossSocketsCoresThreads),
								},
							},
						},
						v1.CPU{Sockets: 3, Cores: 2, Threads: 2},
					),
					Entry("to SocketsCoresThreads with 16 vCPUs and a default ratio of 1:2:2",
						uint32(16),
						instancetypev1beta1.VirtualMachinePreferenceSpec{
							CPU: &instancetypev1beta1.CPUPreferences{
								SpreadOptions: &instancetypev1beta1.SpreadOptions{
									Across: pointer.P(instancetypev1beta1.SpreadAcrossSocketsCoresThreads),
								},
							},
						},
						v1.CPU{Sockets: 4, Cores: 2, Threads: 2},
					),
					Entry("to SocketsCoresThreads with 6 vCPUs and a ratio of 1:3:2",
						uint32(6),
						instancetypev1beta1.VirtualMachinePreferenceSpec{
							CPU: &instancetypev1beta1.CPUPreferences{
								SpreadOptions: &instancetypev1beta1.SpreadOptions{
									Across: pointer.P(instancetypev1beta1.SpreadAcrossSocketsCoresThreads),
									Ratio:  pointer.P(uint32(3)),
								},
							},
						},
						v1.CPU{Sockets: 1, Cores: 3, Threads: 2},
					),
					Entry("to SocketsCoresThreads with 12 vCPUs and a ratio of 1:3:2",
						uint32(12),
						instancetypev1beta1.VirtualMachinePreferenceSpec{
							CPU: &instancetypev1beta1.CPUPreferences{
								SpreadOptions: &instancetypev1beta1.SpreadOptions{
									Across: pointer.P(instancetypev1beta1.SpreadAcrossSocketsCoresThreads),
									Ratio:  pointer.P(uint32(3)),
								},
							},
						},
						v1.CPU{Sockets: 2, Cores: 3, Threads: 2},
					),
					Entry("to SocketsCoresThreads with 18 vCPUs and a ratio of 1:3:2",
						uint32(18),
						instancetypev1beta1.VirtualMachinePreferenceSpec{
							CPU: &instancetypev1beta1.CPUPreferences{
								SpreadOptions: &instancetypev1beta1.SpreadOptions{
									Across: pointer.P(instancetypev1beta1.SpreadAcrossSocketsCoresThreads),
									Ratio:  pointer.P(uint32(3)),
								},
							},
						},
						v1.CPU{Sockets: 3, Cores: 3, Threads: 2},
					),
					Entry("to SocketsCoresThreads with 24 vCPUs and a ratio of 1:3:2",
						uint32(24),
						instancetypev1beta1.VirtualMachinePreferenceSpec{
							CPU: &instancetypev1beta1.CPUPreferences{
								SpreadOptions: &instancetypev1beta1.SpreadOptions{
									Across: pointer.P(instancetypev1beta1.SpreadAcrossSocketsCoresThreads),
									Ratio:  pointer.P(uint32(3)),
								},
							},
						},
						v1.CPU{Sockets: 4, Cores: 3, Threads: 2},
					),
					Entry("to SocketsCoresThreads with 8 vCPUs and a ratio of 1:4:2",
						uint32(8),
						instancetypev1beta1.VirtualMachinePreferenceSpec{
							CPU: &instancetypev1beta1.CPUPreferences{
								SpreadOptions: &instancetypev1beta1.SpreadOptions{
									Across: pointer.P(instancetypev1beta1.SpreadAcrossSocketsCoresThreads),
									Ratio:  pointer.P(uint32(4)),
								},
							},
						},
						v1.CPU{Sockets: 1, Cores: 4, Threads: 2},
					),
					Entry("to SocketsCoresThreads with 16 vCPUs and a ratio of 1:4:2",
						uint32(16),
						instancetypev1beta1.VirtualMachinePreferenceSpec{
							CPU: &instancetypev1beta1.CPUPreferences{
								SpreadOptions: &instancetypev1beta1.SpreadOptions{
									Across: pointer.P(instancetypev1beta1.SpreadAcrossSocketsCoresThreads),
									Ratio:  pointer.P(uint32(4)),
								},
							},
						},
						v1.CPU{Sockets: 2, Cores: 4, Threads: 2},
					),
					Entry("to SocketsCoresThreads with 24 vCPUs and a ratio of 1:4:2",
						uint32(24),
						instancetypev1beta1.VirtualMachinePreferenceSpec{
							CPU: &instancetypev1beta1.CPUPreferences{
								SpreadOptions: &instancetypev1beta1.SpreadOptions{
									Across: pointer.P(instancetypev1beta1.SpreadAcrossSocketsCoresThreads),
									Ratio:  pointer.P(uint32(4)),
								},
							},
						},
						v1.CPU{Sockets: 3, Cores: 4, Threads: 2},
					),
					Entry("to SocketsCoresThreads with 36 vCPUs and a ratio of 1:4:2",
						uint32(36),
						instancetypev1beta1.VirtualMachinePreferenceSpec{
							CPU: &instancetypev1beta1.CPUPreferences{
								SpreadOptions: &instancetypev1beta1.SpreadOptions{
									Across: pointer.P(instancetypev1beta1.SpreadAcrossSocketsCoresThreads),
									Ratio:  pointer.P(uint32(4)),
								},
							},
						},
						v1.CPU{Sockets: 4, Cores: 4, Threads: 2},
					),
					Entry("to CoresThreads with 2 vCPUs and a default ratio of 1:2",
						uint32(2),
						instancetypev1beta1.VirtualMachinePreferenceSpec{
							CPU: &instancetypev1beta1.CPUPreferences{
								SpreadOptions: &instancetypev1beta1.SpreadOptions{
									Across: pointer.P(instancetypev1beta1.SpreadAcrossCoresThreads),
								},
							},
						},
						v1.CPU{Sockets: 1, Cores: 1, Threads: 2},
					),
					Entry("to CoresThreads with 4 vCPUs and a default ratio of 1:2",
						uint32(4),
						instancetypev1beta1.VirtualMachinePreferenceSpec{
							CPU: &instancetypev1beta1.CPUPreferences{
								SpreadOptions: &instancetypev1beta1.SpreadOptions{
									Across: pointer.P(instancetypev1beta1.SpreadAcrossCoresThreads),
								},
							},
						},
						v1.CPU{Sockets: 1, Cores: 2, Threads: 2},
					),
					Entry("to CoresThreads with 6 vCPUs and a default ratio of 1:2",
						uint32(6),
						instancetypev1beta1.VirtualMachinePreferenceSpec{
							CPU: &instancetypev1beta1.CPUPreferences{
								SpreadOptions: &instancetypev1beta1.SpreadOptions{
									Across: pointer.P(instancetypev1beta1.SpreadAcrossCoresThreads),
								},
							},
						},
						v1.CPU{Sockets: 1, Cores: 3, Threads: 2},
					),
					Entry("to CoresThreads with 8 vCPUs and a default ratio of 1:2",
						uint32(8),
						instancetypev1beta1.VirtualMachinePreferenceSpec{
							CPU: &instancetypev1beta1.CPUPreferences{
								SpreadOptions: &instancetypev1beta1.SpreadOptions{
									Across: pointer.P(instancetypev1beta1.SpreadAcrossCoresThreads),
								},
							},
						},
						v1.CPU{Sockets: 1, Cores: 4, Threads: 2},
					),
				)
			})

			It("should return a conflict if vmi.Spec.Domain.CPU already defined", func() {
				instancetypeSpec = &instancetypev1beta1.VirtualMachineInstancetypeSpec{
					CPU: instancetypev1beta1.CPUInstancetype{
						Guest: uint32(2),
					},
				}

				vmi.Spec.Domain.CPU = &v1.CPU{
					Cores:   4,
					Sockets: 1,
					Threads: 1,
				}

				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(HaveLen(3))
				Expect(conflicts[0].String()).To(Equal("spec.template.spec.domain.cpu.sockets"))
				Expect(conflicts[1].String()).To(Equal("spec.template.spec.domain.cpu.cores"))
				Expect(conflicts[2].String()).To(Equal("spec.template.spec.domain.cpu.threads"))
			})

			It("should return a conflict if vmi.Spec.Domain.Resources.Requests[k8sv1.ResourceCPU] already defined", func() {
				instancetypeSpec = &instancetypev1beta1.VirtualMachineInstancetypeSpec{
					CPU: instancetypev1beta1.CPUInstancetype{
						Guest: uint32(2),
					},
				}

				vmi.Spec.Domain.Resources = v1.ResourceRequirements{
					Requests: k8sv1.ResourceList{
						k8sv1.ResourceCPU: resource.MustParse("1"),
					},
				}

				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(HaveLen(1))
				Expect(conflicts[0].String()).To(Equal("spec.template.spec.domain.resources.requests.cpu"))
			})

			It("should return a conflict if vmi.Spec.Domain.Resources.Limits[k8sv1.ResourceCPU] already defined", func() {
				instancetypeSpec = &instancetypev1beta1.VirtualMachineInstancetypeSpec{
					CPU: instancetypev1beta1.CPUInstancetype{
						Guest: uint32(2),
					},
				}

				vmi.Spec.Domain.Resources = v1.ResourceRequirements{
					Limits: k8sv1.ResourceList{
						k8sv1.ResourceCPU: resource.MustParse("1"),
					},
				}

				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(HaveLen(1))
				Expect(conflicts[0].String()).To(Equal("spec.template.spec.domain.resources.limits.cpu"))
			})

			It("should apply PreferredCPUFeatures", func() {
				preferenceSpec = &instancetypev1beta1.VirtualMachinePreferenceSpec{
					CPU: &instancetypev1beta1.CPUPreferences{
						PreferredCPUFeatures: []v1.CPUFeature{
							{
								Name:   "foo",
								Policy: "require",
							},
							{
								Name:   "bar",
								Policy: "force",
							},
						},
					},
				}
				vmi.Spec.Domain.CPU = &v1.CPU{
					Features: []v1.CPUFeature{
						{
							Name:   "bar",
							Policy: "optional",
						},
					},
				}
				Expect(instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)).To(BeEmpty())
				Expect(vmi.Spec.Domain.CPU.Features).To(HaveLen(2))
				Expect(vmi.Spec.Domain.CPU.Features).To(ContainElements([]v1.CPUFeature{
					{
						Name:   "foo",
						Policy: "require",
					},
					{
						Name:   "bar",
						Policy: "optional",
					},
				}))
			})
		})

		Context("instancetype.Spec.Memory", func() {
			BeforeEach(func() {
				maxGuest := resource.MustParse("2G")
				instancetypeSpec = &instancetypev1beta1.VirtualMachineInstancetypeSpec{
					Memory: instancetypev1beta1.MemoryInstancetype{
						Guest: resource.MustParse("512M"),
						Hugepages: &v1.Hugepages{
							PageSize: "1Gi",
						},
						MaxGuest: &maxGuest,
					},
				}
			})

			It("should apply to VMI", func() {
				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(BeEmpty())

				Expect(*vmi.Spec.Domain.Memory.Guest).To(Equal(instancetypeSpec.Memory.Guest))
				Expect(*vmi.Spec.Domain.Memory.Hugepages).To(Equal(*instancetypeSpec.Memory.Hugepages))
				Expect(vmi.Spec.Domain.Memory.MaxGuest.Equal(*instancetypeSpec.Memory.MaxGuest)).To(BeTrue())
			})

			It("should apply memory overcommit correctly to VMI", func() {
				instancetypeSpec.Memory.Hugepages = nil
				instancetypeSpec.Memory.OvercommitPercent = 15

				expectedOverhead := int64(float32(instancetypeSpec.Memory.Guest.Value()) * (1 - float32(instancetypeSpec.Memory.OvercommitPercent)/100))
				Expect(expectedOverhead).ToNot(Equal(instancetypeSpec.Memory.Guest.Value()))

				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(BeEmpty())
				memRequest := vmi.Spec.Domain.Resources.Requests[k8sv1.ResourceMemory]
				Expect(memRequest.Value()).To(Equal(expectedOverhead))
			})

			It("should detect memory conflict", func() {
				vmiMemGuest := resource.MustParse("512M")
				vmi.Spec.Domain.Memory = &v1.Memory{
					Guest: &vmiMemGuest,
				}

				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(HaveLen(1))
				Expect(conflicts[0].String()).To(Equal("spec.template.spec.domain.memory"))
			})

			It("should return a conflict if vmi.Spec.Domain.Resources.Requests[k8sv1.ResourceMemory] already defined", func() {
				instancetypeSpec = &instancetypev1beta1.VirtualMachineInstancetypeSpec{
					Memory: instancetypev1beta1.MemoryInstancetype{
						Guest: resource.MustParse("512M"),
					},
				}

				vmi.Spec.Domain.Resources = v1.ResourceRequirements{
					Requests: k8sv1.ResourceList{
						k8sv1.ResourceMemory: resource.MustParse("128Mi"),
					},
				}

				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(HaveLen(1))
				Expect(conflicts[0].String()).To(Equal("spec.template.spec.domain.resources.requests.memory"))
			})

			It("should return a conflict if vmi.Spec.Domain.Resources.Limits[k8sv1.ResourceMemory] already defined", func() {
				instancetypeSpec = &instancetypev1beta1.VirtualMachineInstancetypeSpec{
					Memory: instancetypev1beta1.MemoryInstancetype{
						Guest: resource.MustParse("512M"),
					},
				}

				vmi.Spec.Domain.Resources = v1.ResourceRequirements{
					Limits: k8sv1.ResourceList{
						k8sv1.ResourceMemory: resource.MustParse("128Mi"),
					},
				}

				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(HaveLen(1))
				Expect(conflicts[0].String()).To(Equal("spec.template.spec.domain.resources.limits.memory"))
			})
		})
		Context("instancetype.Spec.ioThreadsPolicy", func() {
			var instancetypePolicy v1.IOThreadsPolicy

			BeforeEach(func() {
				instancetypePolicy = v1.IOThreadsPolicyShared
				instancetypeSpec = &instancetypev1beta1.VirtualMachineInstancetypeSpec{
					IOThreadsPolicy: &instancetypePolicy,
				}
			})

			It("should apply to VMI", func() {
				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(BeEmpty())

				Expect(*vmi.Spec.Domain.IOThreadsPolicy).To(Equal(*instancetypeSpec.IOThreadsPolicy))
			})

			It("should detect IOThreadsPolicy conflict", func() {
				vmi.Spec.Domain.IOThreadsPolicy = &instancetypePolicy

				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(HaveLen(1))
				Expect(conflicts[0].String()).To(Equal("spec.template.spec.domain.ioThreadsPolicy"))
			})
		})

		Context("instancetype.Spec.LaunchSecurity", func() {
			BeforeEach(func() {
				instancetypeSpec = &instancetypev1beta1.VirtualMachineInstancetypeSpec{
					LaunchSecurity: &v1.LaunchSecurity{
						SEV: &v1.SEV{},
					},
				}
			})

			It("should apply to VMI", func() {
				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(BeEmpty())

				Expect(*vmi.Spec.Domain.LaunchSecurity).To(Equal(*instancetypeSpec.LaunchSecurity))
			})

			It("should detect LaunchSecurity conflict", func() {
				vmi.Spec.Domain.LaunchSecurity = &v1.LaunchSecurity{
					SEV: &v1.SEV{},
				}

				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(HaveLen(1))
				Expect(conflicts[0].String()).To(Equal("spec.template.spec.domain.launchSecurity"))
			})
		})

		Context("instancetype.Spec.GPUs", func() {
			BeforeEach(func() {
				instancetypeSpec = &instancetypev1beta1.VirtualMachineInstancetypeSpec{
					GPUs: []v1.GPU{
						{
							Name:       "barfoo",
							DeviceName: "vendor.com/gpu_name",
						},
					},
				}
			})

			It("should apply to VMI", func() {
				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(BeEmpty())

				Expect(vmi.Spec.Domain.Devices.GPUs).To(Equal(instancetypeSpec.GPUs))
			})

			It("should detect GPU conflict", func() {
				vmi.Spec.Domain.Devices.GPUs = []v1.GPU{
					{
						Name:       "foobar",
						DeviceName: "vendor.com/gpu_name",
					},
				}

				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(HaveLen(1))
				Expect(conflicts[0].String()).To(Equal("spec.template.spec.domain.devices.gpus"))
			})
		})

		Context("instancetype.Spec.HostDevices", func() {
			BeforeEach(func() {
				instancetypeSpec = &instancetypev1beta1.VirtualMachineInstancetypeSpec{
					HostDevices: []v1.HostDevice{
						{
							Name:       "foobar",
							DeviceName: "vendor.com/device_name",
						},
					},
				}
			})

			It("should apply to VMI", func() {
				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(BeEmpty())

				Expect(vmi.Spec.Domain.Devices.HostDevices).To(Equal(instancetypeSpec.HostDevices))
			})

			It("should detect HostDevice conflict", func() {
				vmi.Spec.Domain.Devices.HostDevices = []v1.HostDevice{
					{
						Name:       "foobar",
						DeviceName: "vendor.com/device_name",
					},
				}

				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(HaveLen(1))
				Expect(conflicts[0].String()).To(Equal("spec.template.spec.domain.devices.hostDevices"))
			})
		})

		Context("Instancetype.Spec.Annotations", func() {
			var multipleAnnotations map[string]string

			BeforeEach(func() {
				multipleAnnotations = map[string]string{
					"annotation-1": "1",
					"annotation-2": "2",
				}

				instancetypeSpec = &instancetypev1beta1.VirtualMachineInstancetypeSpec{
					Annotations: make(map[string]string),
				}
			})

			It("should apply to VMI", func() {
				instancetypeSpec.Annotations = multipleAnnotations

				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, nil, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(BeEmpty())
				Expect(vmi.Annotations).To(Equal(instancetypeSpec.Annotations))
			})

			It("should not detect conflict when annotation with the same value already exists", func() {
				instancetypeSpec.Annotations = multipleAnnotations
				vmi.Annotations = multipleAnnotations

				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, nil, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(BeEmpty())
				Expect(vmi.Annotations).To(Equal(instancetypeSpec.Annotations))
			})

			It("should detect conflict when annotation with different value already exists", func() {
				instancetypeSpec.Annotations = multipleAnnotations
				vmi.Annotations = map[string]string{
					"annotation-1": "conflict",
				}

				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, nil, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(HaveLen(1))
				Expect(conflicts[0].String()).To(Equal("annotations.annotation-1"))
			})
		})

		Context("Preference.Spec.Annotations", func() {
			var multipleAnnotations map[string]string

			BeforeEach(func() {
				multipleAnnotations = map[string]string{
					"annotation-1": "1",
					"annotation-2": "2",
				}

				preferenceSpec = &instancetypev1beta1.VirtualMachinePreferenceSpec{
					Annotations: make(map[string]string),
				}
			})

			It("should apply to VMI", func() {
				preferenceSpec.Annotations = multipleAnnotations

				conflicts := instancetypeMethods.ApplyToVmi(field, nil, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(BeEmpty())
				Expect(vmi.Annotations).To(Equal(preferenceSpec.Annotations))
			})

			It("should not overwrite already existing values", func() {
				preferenceSpec.Annotations = multipleAnnotations
				vmiAnnotations := map[string]string{
					"annotation-1": "dont-overwrite",
					"annotation-2": "dont-overwrite",
					"annotation-3": "dont-overwrite",
				}
				vmi.Annotations = vmiAnnotations

				conflicts := instancetypeMethods.ApplyToVmi(field, nil, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(BeEmpty())
				Expect(vmi.Annotations).To(HaveLen(3))
				Expect(vmi.Annotations).To(Equal(vmiAnnotations))
			})
		})

		// TODO - break this up into smaller more targeted tests
		Context("Preference.Devices", func() {
			var userDefinedBlockSize *v1.BlockSize

			BeforeEach(func() {
				userDefinedBlockSize = &v1.BlockSize{
					Custom: &v1.CustomBlockSize{
						Logical:  512,
						Physical: 512,
					},
				}
				vmi.Spec.Domain.Devices.AutoattachGraphicsDevice = pointer.P(false)
				vmi.Spec.Domain.Devices.AutoattachMemBalloon = pointer.P(false)
				vmi.Spec.Domain.Devices.Disks = []v1.Disk{
					{
						Cache:     v1.CacheWriteBack,
						IO:        v1.IONative,
						BlockSize: userDefinedBlockSize,
						DiskDevice: v1.DiskDevice{
							Disk: &v1.DiskTarget{
								Bus: v1.DiskBusSCSI,
							},
						},
					},
					{
						DiskDevice: v1.DiskDevice{
							Disk: &v1.DiskTarget{},
						},
					},
					{
						DiskDevice: v1.DiskDevice{
							CDRom: &v1.CDRomTarget{
								Bus: v1.DiskBusSATA,
							},
						},
					},
					{
						DiskDevice: v1.DiskDevice{
							CDRom: &v1.CDRomTarget{},
						},
					},
					{
						DiskDevice: v1.DiskDevice{
							LUN: &v1.LunTarget{
								Bus: v1.DiskBusSATA,
							},
						},
					},
					{
						DiskDevice: v1.DiskDevice{
							LUN: &v1.LunTarget{},
						},
					},
				}
				vmi.Spec.Domain.Devices.Inputs = []v1.Input{
					{
						Bus:  "usb",
						Type: "tablet",
					},
					{},
				}
				vmi.Spec.Domain.Devices.Interfaces = []v1.Interface{
					{
						Name:  "primary",
						Model: "e1000",
					},
					{
						Name: "secondary",
					},
				}
				vmi.Spec.Domain.Devices.Sound = &v1.SoundDevice{}

				preferenceSpec = &instancetypev1beta1.VirtualMachinePreferenceSpec{
					Devices: &instancetypev1beta1.DevicePreferences{
						PreferredAutoattachGraphicsDevice:   pointer.P(true),
						PreferredAutoattachMemBalloon:       pointer.P(true),
						PreferredAutoattachPodInterface:     pointer.P(true),
						PreferredAutoattachSerialConsole:    pointer.P(true),
						PreferredAutoattachInputDevice:      pointer.P(true),
						PreferredDiskDedicatedIoThread:      pointer.P(true),
						PreferredDisableHotplug:             pointer.P(true),
						PreferredUseVirtioTransitional:      pointer.P(true),
						PreferredNetworkInterfaceMultiQueue: pointer.P(true),
						PreferredBlockMultiQueue:            pointer.P(true),
						PreferredDiskBlockSize: &v1.BlockSize{
							Custom: &v1.CustomBlockSize{
								Logical:  4096,
								Physical: 4096,
							},
						},
						PreferredDiskCache:           v1.CacheWriteThrough,
						PreferredDiskIO:              v1.IONative,
						PreferredDiskBus:             v1.DiskBusVirtio,
						PreferredCdromBus:            v1.DiskBusSCSI,
						PreferredLunBus:              v1.DiskBusSATA,
						PreferredInputBus:            v1.InputBusVirtio,
						PreferredInputType:           v1.InputTypeTablet,
						PreferredInterfaceModel:      v1.VirtIO,
						PreferredSoundModel:          "ac97",
						PreferredRng:                 &v1.Rng{},
						PreferredTPM:                 &v1.TPMDevice{},
						PreferredInterfaceMasquerade: &v1.InterfaceMasquerade{},
					},
				}
			})

			Context("PreferredInterfaceMasquerade", func() {
				It("should be applied to interface on Pod network", func() {
					vmi.Spec.Networks = []v1.Network{{
						Name: vmi.Spec.Domain.Devices.Interfaces[0].Name,
						NetworkSource: v1.NetworkSource{
							Pod: &v1.PodNetwork{},
						},
					}}
					Expect(instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)).To(Succeed())
					Expect(vmi.Spec.Domain.Devices.Interfaces[0].Masquerade).ToNot(BeNil())
					Expect(vmi.Spec.Domain.Devices.Interfaces[1].Masquerade).To(BeNil())
				})
				It("should not be applied on interface that has another binding set", func() {
					vmi.Spec.Domain.Devices.Interfaces[0].SRIOV = &v1.InterfaceSRIOV{}
					Expect(instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)).To(Succeed())
					Expect(vmi.Spec.Domain.Devices.Interfaces[0].Masquerade).To(BeNil())
					Expect(vmi.Spec.Domain.Devices.Interfaces[0].SRIOV).ToNot(BeNil())
				})
				It("should not be applied on interface that is not on Pod network", func() {
					vmi.Spec.Networks = []v1.Network{{
						Name: vmi.Spec.Domain.Devices.Interfaces[0].Name,
					}}
					Expect(instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)).To(Succeed())
					Expect(vmi.Spec.Domain.Devices.Interfaces[0].Masquerade).To(BeNil())
				})
			})

			It("should apply to VMI", func() {
				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(BeEmpty())

				Expect(*vmi.Spec.Domain.Devices.AutoattachGraphicsDevice).To(BeFalse())
				Expect(*vmi.Spec.Domain.Devices.AutoattachMemBalloon).To(BeFalse())
				Expect(*vmi.Spec.Domain.Devices.AutoattachInputDevice).To(BeTrue())
				Expect(vmi.Spec.Domain.Devices.Disks[0].Cache).To(Equal(v1.CacheWriteBack))
				Expect(vmi.Spec.Domain.Devices.Disks[0].IO).To(Equal(v1.IONative))
				Expect(*vmi.Spec.Domain.Devices.Disks[0].BlockSize).To(Equal(*userDefinedBlockSize))
				Expect(vmi.Spec.Domain.Devices.Disks[0].DiskDevice.Disk.Bus).To(Equal(v1.DiskBusSCSI))
				Expect(vmi.Spec.Domain.Devices.Disks[2].DiskDevice.CDRom.Bus).To(Equal(v1.DiskBusSATA))
				Expect(vmi.Spec.Domain.Devices.Disks[4].DiskDevice.LUN.Bus).To(Equal(v1.DiskBusSATA))
				Expect(vmi.Spec.Domain.Devices.Inputs[0].Bus).To(Equal(v1.InputBusUSB))
				Expect(vmi.Spec.Domain.Devices.Inputs[0].Type).To(Equal(v1.InputTypeTablet))
				Expect(vmi.Spec.Domain.Devices.Interfaces[0].Model).To(Equal("e1000"))

				// Assert that everything that isn't defined in the VM/VMI should use Preferences
				Expect(*vmi.Spec.Domain.Devices.AutoattachPodInterface).To(Equal(*preferenceSpec.Devices.PreferredAutoattachPodInterface))
				Expect(*vmi.Spec.Domain.Devices.AutoattachSerialConsole).To(Equal(*preferenceSpec.Devices.PreferredAutoattachSerialConsole))
				Expect(vmi.Spec.Domain.Devices.DisableHotplug).To(Equal(*preferenceSpec.Devices.PreferredDisableHotplug))
				Expect(*vmi.Spec.Domain.Devices.UseVirtioTransitional).To(Equal(*preferenceSpec.Devices.PreferredUseVirtioTransitional))
				Expect(vmi.Spec.Domain.Devices.Disks[1].Cache).To(Equal(preferenceSpec.Devices.PreferredDiskCache))
				Expect(vmi.Spec.Domain.Devices.Disks[1].IO).To(Equal(preferenceSpec.Devices.PreferredDiskIO))
				Expect(*vmi.Spec.Domain.Devices.Disks[1].BlockSize).To(Equal(*preferenceSpec.Devices.PreferredDiskBlockSize))
				Expect(vmi.Spec.Domain.Devices.Disks[1].DiskDevice.Disk.Bus).To(Equal(preferenceSpec.Devices.PreferredDiskBus))
				Expect(vmi.Spec.Domain.Devices.Disks[3].DiskDevice.CDRom.Bus).To(Equal(preferenceSpec.Devices.PreferredCdromBus))
				Expect(vmi.Spec.Domain.Devices.Disks[5].DiskDevice.LUN.Bus).To(Equal(preferenceSpec.Devices.PreferredLunBus))
				Expect(vmi.Spec.Domain.Devices.Inputs[1].Bus).To(Equal(preferenceSpec.Devices.PreferredInputBus))
				Expect(vmi.Spec.Domain.Devices.Inputs[1].Type).To(Equal(preferenceSpec.Devices.PreferredInputType))
				Expect(vmi.Spec.Domain.Devices.Interfaces[1].Model).To(Equal(preferenceSpec.Devices.PreferredInterfaceModel))
				Expect(vmi.Spec.Domain.Devices.Sound.Model).To(Equal(preferenceSpec.Devices.PreferredSoundModel))
				Expect(*vmi.Spec.Domain.Devices.Rng).To(Equal(*preferenceSpec.Devices.PreferredRng))
				Expect(*vmi.Spec.Domain.Devices.NetworkInterfaceMultiQueue).To(Equal(*preferenceSpec.Devices.PreferredNetworkInterfaceMultiQueue))
				Expect(*vmi.Spec.Domain.Devices.BlockMultiQueue).To(Equal(*preferenceSpec.Devices.PreferredBlockMultiQueue))
				Expect(*vmi.Spec.Domain.Devices.TPM).To(Equal(*preferenceSpec.Devices.PreferredTPM))
			})

			It("Should apply when a VMI disk doesn't have a DiskDevice target defined", func() {
				vmi.Spec.Domain.Devices.Disks[1].DiskDevice.Disk = nil

				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(BeEmpty())

				Expect(vmi.Spec.Domain.Devices.Disks[1].DiskDevice.Disk.Bus).To(Equal(preferenceSpec.Devices.PreferredDiskBus))
			})

			It("[test_id:CNV-9817] Should ignore preference when a VMI disk have a DiskDevice defined", func() {
				diskTypeForTest := v1.DiskBusSCSI

				vmi.Spec.Domain.Devices.Disks[1].DiskDevice.Disk.Bus = diskTypeForTest
				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(BeEmpty())

				Expect(vmi.Spec.Domain.Devices.Disks[1].DiskDevice.Disk.Bus).To(Equal(diskTypeForTest))
			})

			Context("PreferredDiskDedicatedIoThread", func() {
				DescribeTable("should be ignored when", func(preferenceSpec *instancetypev1beta1.VirtualMachinePreferenceSpec) {
					Expect(instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)).To(BeEmpty())
					for _, disk := range vmi.Spec.Domain.Devices.Disks {
						if disk.DiskDevice.Disk != nil {
							Expect(disk.DedicatedIOThread).To(BeNil())
						}
					}
				},
					Entry("unset", &instancetypev1beta1.VirtualMachinePreferenceSpec{
						Devices: &instancetypev1beta1.DevicePreferences{},
					}),
					Entry("false", &instancetypev1beta1.VirtualMachinePreferenceSpec{
						Devices: &instancetypev1beta1.DevicePreferences{
							PreferredDiskDedicatedIoThread: pointer.P(false),
						},
					}),
				)
				It("should only apply to virtio disk devices", func() {
					preferenceSpec = &instancetypev1beta1.VirtualMachinePreferenceSpec{
						Devices: &instancetypev1beta1.DevicePreferences{
							PreferredDiskDedicatedIoThread: pointer.P(true),
						},
					}
					Expect(instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)).To(BeEmpty())
					for _, disk := range vmi.Spec.Domain.Devices.Disks {
						if disk.DiskDevice.Disk != nil {
							if disk.DiskDevice.Disk.Bus == v1.DiskBusVirtio {
								Expect(disk.DedicatedIOThread).To(HaveValue(BeTrue()))
							} else {
								Expect(disk.DedicatedIOThread).To(BeNil())
							}
						}
					}
				})
			})
		})

		Context("Preference.Features", func() {
			BeforeEach(func() {
				spinLockRetries := uint32(32)
				preferenceSpec = &instancetypev1beta1.VirtualMachinePreferenceSpec{
					Features: &instancetypev1beta1.FeaturePreferences{
						PreferredAcpi: &v1.FeatureState{},
						PreferredApic: &v1.FeatureAPIC{
							Enabled:        pointer.P(true),
							EndOfInterrupt: false,
						},
						PreferredHyperv: &v1.FeatureHyperv{
							Relaxed: &v1.FeatureState{},
							VAPIC:   &v1.FeatureState{},
							Spinlocks: &v1.FeatureSpinlocks{
								Enabled: pointer.P(true),
								Retries: &spinLockRetries,
							},
							VPIndex: &v1.FeatureState{},
							Runtime: &v1.FeatureState{},
							SyNIC:   &v1.FeatureState{},
							SyNICTimer: &v1.SyNICTimer{
								Enabled: pointer.P(true),
								Direct:  &v1.FeatureState{},
							},
							Reset: &v1.FeatureState{},
							VendorID: &v1.FeatureVendorID{
								Enabled:  pointer.P(true),
								VendorID: "1234",
							},
							Frequencies:     &v1.FeatureState{},
							Reenlightenment: &v1.FeatureState{},
							TLBFlush:        &v1.FeatureState{},
							IPI:             &v1.FeatureState{},
							EVMCS:           &v1.FeatureState{},
						},
						PreferredKvm: &v1.FeatureKVM{
							Hidden: true,
						},
						PreferredPvspinlock: &v1.FeatureState{},
						PreferredSmm:        &v1.FeatureState{},
					},
				}
			})

			It("should apply to VMI", func() {
				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(BeEmpty())

				Expect(vmi.Spec.Domain.Features.ACPI).To(Equal(*preferenceSpec.Features.PreferredAcpi))
				Expect(*vmi.Spec.Domain.Features.APIC).To(Equal(*preferenceSpec.Features.PreferredApic))
				Expect(*vmi.Spec.Domain.Features.Hyperv).To(Equal(*preferenceSpec.Features.PreferredHyperv))
				Expect(*vmi.Spec.Domain.Features.KVM).To(Equal(*preferenceSpec.Features.PreferredKvm))
				Expect(*vmi.Spec.Domain.Features.Pvspinlock).To(Equal(*preferenceSpec.Features.PreferredPvspinlock))
				Expect(*vmi.Spec.Domain.Features.SMM).To(Equal(*preferenceSpec.Features.PreferredSmm))
			})

			It("should apply when some HyperV features already defined in the VMI", func() {
				vmi.Spec.Domain.Features = &v1.Features{
					Hyperv: &v1.FeatureHyperv{
						EVMCS: &v1.FeatureState{
							Enabled: pointer.P(false),
						},
					},
				}

				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(BeEmpty())

				Expect(*vmi.Spec.Domain.Features.Hyperv.EVMCS.Enabled).To(BeFalse())
			})
		})

		Context("Preference.Firmware", func() {
			It("should apply BIOS preferences full to VMI", func() {
				preferenceSpec = &instancetypev1beta1.VirtualMachinePreferenceSpec{
					Firmware: &instancetypev1beta1.FirmwarePreferences{
						PreferredUseBios:                 pointer.P(true),
						PreferredUseBiosSerial:           pointer.P(true),
						DeprecatedPreferredUseEfi:        pointer.P(false),
						DeprecatedPreferredUseSecureBoot: pointer.P(false),
					},
				}

				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(BeEmpty())

				Expect(*vmi.Spec.Domain.Firmware.Bootloader.BIOS.UseSerial).To(Equal(*preferenceSpec.Firmware.PreferredUseBiosSerial))
			})

			It("should apply SecureBoot preferences full to VMI", func() {
				preferenceSpec = &instancetypev1beta1.VirtualMachinePreferenceSpec{
					Firmware: &instancetypev1beta1.FirmwarePreferences{
						PreferredUseBios:                 pointer.P(false),
						PreferredUseBiosSerial:           pointer.P(false),
						DeprecatedPreferredUseEfi:        pointer.P(true),
						DeprecatedPreferredUseSecureBoot: pointer.P(true),
					},
				}

				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(BeEmpty())

				Expect(*vmi.Spec.Domain.Firmware.Bootloader.EFI.SecureBoot).To(Equal(*preferenceSpec.Firmware.DeprecatedPreferredUseSecureBoot))
			})

			It("should not overwrite user defined Bootloader.BIOS with DeprecatedPreferredUseEfi - bug #10313", func() {
				preferenceSpec = &instancetypev1beta1.VirtualMachinePreferenceSpec{
					Firmware: &instancetypev1beta1.FirmwarePreferences{
						DeprecatedPreferredUseEfi:        pointer.P(true),
						DeprecatedPreferredUseSecureBoot: pointer.P(true),
					},
				}
				vmi.Spec.Domain.Firmware = &v1.Firmware{
					Bootloader: &v1.Bootloader{
						BIOS: &v1.BIOS{
							UseSerial: pointer.P(false),
						},
					},
				}
				Expect(instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)).To(BeEmpty())
				Expect(vmi.Spec.Domain.Firmware.Bootloader.EFI).To(BeNil())
				Expect(*vmi.Spec.Domain.Firmware.Bootloader.BIOS.UseSerial).To(BeFalse())
			})

			It("should not overwrite user defined value with PreferredUseBiosSerial - bug #10313", func() {
				preferenceSpec = &instancetypev1beta1.VirtualMachinePreferenceSpec{
					Firmware: &instancetypev1beta1.FirmwarePreferences{
						PreferredUseBios:       pointer.P(true),
						PreferredUseBiosSerial: pointer.P(true),
					},
				}
				vmi.Spec.Domain.Firmware = &v1.Firmware{
					Bootloader: &v1.Bootloader{
						BIOS: &v1.BIOS{
							UseSerial: pointer.P(false),
						},
					},
				}
				Expect(instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)).To(BeEmpty())
				Expect(*vmi.Spec.Domain.Firmware.Bootloader.BIOS.UseSerial).To(BeFalse())
			})

			It("should not overwrite user defined Bootloader.EFI with PreferredUseBios - bug #10313", func() {
				preferenceSpec = &instancetypev1beta1.VirtualMachinePreferenceSpec{
					Firmware: &instancetypev1beta1.FirmwarePreferences{
						PreferredUseBios:       pointer.P(true),
						PreferredUseBiosSerial: pointer.P(true),
					},
				}
				vmi.Spec.Domain.Firmware = &v1.Firmware{
					Bootloader: &v1.Bootloader{
						EFI: &v1.EFI{
							SecureBoot: pointer.P(false),
						},
					},
				}
				Expect(instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)).To(BeEmpty())
				Expect(vmi.Spec.Domain.Firmware.Bootloader.BIOS).To(BeNil())
				Expect(*vmi.Spec.Domain.Firmware.Bootloader.EFI.SecureBoot).To(BeFalse())
			})

			It("should not overwrite user defined value with DeprecatedPreferredUseSecureBoot - bug #10313", func() {
				preferenceSpec = &instancetypev1beta1.VirtualMachinePreferenceSpec{
					Firmware: &instancetypev1beta1.FirmwarePreferences{
						DeprecatedPreferredUseEfi:        pointer.P(true),
						DeprecatedPreferredUseSecureBoot: pointer.P(true),
					},
				}
				vmi.Spec.Domain.Firmware = &v1.Firmware{
					Bootloader: &v1.Bootloader{
						EFI: &v1.EFI{
							SecureBoot: pointer.P(false),
						},
					},
				}
				Expect(instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)).To(BeEmpty())
				Expect(*vmi.Spec.Domain.Firmware.Bootloader.EFI.SecureBoot).To(BeFalse())
			})

			It("should apply PreferredEfi", func() {
				preferenceSpec = &instancetypev1beta1.VirtualMachinePreferenceSpec{
					Firmware: &instancetypev1beta1.FirmwarePreferences{
						PreferredEfi: &v1.EFI{
							Persistent: pointer.P(true),
							SecureBoot: pointer.P(true),
						},
					},
				}
				Expect(instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)).To(BeEmpty())
				Expect(*vmi.Spec.Domain.Firmware.Bootloader.EFI).ToNot(BeNil())
				Expect(*vmi.Spec.Domain.Firmware.Bootloader.EFI.Persistent).To(BeTrue())
				Expect(*vmi.Spec.Domain.Firmware.Bootloader.EFI.SecureBoot).To(BeTrue())
			})

			It("should ignore DeprecatedPreferredUseEfi and DeprecatedPreferredUseSecureBoot when using PreferredEfi", func() {
				preferenceSpec = &instancetypev1beta1.VirtualMachinePreferenceSpec{
					Firmware: &instancetypev1beta1.FirmwarePreferences{
						PreferredEfi: &v1.EFI{
							Persistent: pointer.P(true),
						},
						DeprecatedPreferredUseEfi:        pointer.P(false),
						DeprecatedPreferredUseSecureBoot: pointer.P(false),
					},
				}
				Expect(instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)).To(BeEmpty())
				Expect(*vmi.Spec.Domain.Firmware.Bootloader.EFI).ToNot(BeNil())
				Expect(*vmi.Spec.Domain.Firmware.Bootloader.EFI.Persistent).To(BeTrue())
				Expect(vmi.Spec.Domain.Firmware.Bootloader.EFI.SecureBoot).To(BeNil())
			})

			It("should not overwrite EFI when using PreferredEfi - bug #12985", func() {
				vmi.Spec.Domain.Firmware = &v1.Firmware{
					Bootloader: &v1.Bootloader{
						EFI: &v1.EFI{
							SecureBoot: pointer.P(false),
						},
					},
				}
				preferenceSpec = &instancetypev1beta1.VirtualMachinePreferenceSpec{
					Firmware: &instancetypev1beta1.FirmwarePreferences{
						PreferredEfi: &v1.EFI{
							SecureBoot: pointer.P(true),
							Persistent: pointer.P(true),
						},
					},
				}
				Expect(instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)).To(BeEmpty())
				Expect(vmi.Spec.Domain.Firmware.Bootloader.EFI).ToNot(BeNil())
				Expect(vmi.Spec.Domain.Firmware.Bootloader.EFI.SecureBoot).ToNot(BeNil())
				Expect(*vmi.Spec.Domain.Firmware.Bootloader.EFI.SecureBoot).To(BeFalse())
				Expect(vmi.Spec.Domain.Firmware.Bootloader.EFI.Persistent).To(BeNil())
			})

			It("should not apply PreferredEfi when VM already using BIOS - bug #12985", func() {
				vmi.Spec.Domain.Firmware = &v1.Firmware{
					Bootloader: &v1.Bootloader{
						BIOS: &v1.BIOS{},
					},
				}
				preferenceSpec = &instancetypev1beta1.VirtualMachinePreferenceSpec{
					Firmware: &instancetypev1beta1.FirmwarePreferences{
						PreferredEfi: &v1.EFI{},
					},
				}
				Expect(instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)).To(BeEmpty())
				Expect(vmi.Spec.Domain.Firmware.Bootloader.BIOS).ToNot(BeNil())
				Expect(vmi.Spec.Domain.Firmware.Bootloader.EFI).To(BeNil())
			})
		})

		Context("Preference.Machine", func() {
			It("should apply to VMI", func() {
				preferenceSpec = &instancetypev1beta1.VirtualMachinePreferenceSpec{
					Machine: &instancetypev1beta1.MachinePreferences{
						PreferredMachineType: "q35-rhel-8.0",
					},
				}

				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(BeEmpty())

				Expect(vmi.Spec.Domain.Machine.Type).To(Equal(preferenceSpec.Machine.PreferredMachineType))
			})
		})
		Context("Preference.Clock", func() {
			It("should apply to VMI", func() {
				preferenceSpec = &instancetypev1beta1.VirtualMachinePreferenceSpec{
					Clock: &instancetypev1beta1.ClockPreferences{
						PreferredClockOffset: &v1.ClockOffset{
							UTC: &v1.ClockOffsetUTC{
								OffsetSeconds: pointer.P(30),
							},
						},
						PreferredTimer: &v1.Timer{
							Hyperv: &v1.HypervTimer{},
						},
					},
				}

				conflicts := instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
				Expect(conflicts).To(BeEmpty())

				Expect(vmi.Spec.Domain.Clock.ClockOffset).To(Equal(*preferenceSpec.Clock.PreferredClockOffset))
				Expect(*vmi.Spec.Domain.Clock.Timer).To(Equal(*preferenceSpec.Clock.PreferredTimer))
			})
		})

		Context("Preference.PreferredSubdomain", func() {
			It("should apply to VMI", func() {
				preferenceSpec = &instancetypev1beta1.VirtualMachinePreferenceSpec{
					PreferredSubdomain: pointer.P("kubevirt.io"),
				}
				Expect(instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)).To(BeEmpty())
				Expect(vmi.Spec.Subdomain).To(Equal(*preferenceSpec.PreferredSubdomain))
			})

			It("should not overwrite user defined value", func() {
				const userDefinedValue = "foo.com"
				vmi.Spec.Subdomain = userDefinedValue
				preferenceSpec = &instancetypev1beta1.VirtualMachinePreferenceSpec{
					PreferredSubdomain: pointer.P("kubevirt.io"),
				}
				Expect(instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)).To(BeEmpty())
				Expect(vmi.Spec.Subdomain).To(Equal(userDefinedValue))
			})
		})

		Context("Preference.PreferredTerminationGracePeriodSeconds", func() {
			It("should apply to VMI", func() {
				preferenceSpec = &instancetypev1beta1.VirtualMachinePreferenceSpec{
					PreferredTerminationGracePeriodSeconds: pointer.P(int64(180)),
				}
				Expect(instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)).To(BeEmpty())
				Expect(*vmi.Spec.TerminationGracePeriodSeconds).To(Equal(*preferenceSpec.PreferredTerminationGracePeriodSeconds))
			})

			It("should not overwrite user defined value", func() {
				const userDefinedValue = int64(100)
				vmi.Spec.TerminationGracePeriodSeconds = pointer.P(userDefinedValue)
				preferenceSpec = &instancetypev1beta1.VirtualMachinePreferenceSpec{
					PreferredTerminationGracePeriodSeconds: pointer.P(int64(180)),
				}
				Expect(instancetypeMethods.ApplyToVmi(field, instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)).To(BeEmpty())
				Expect(*vmi.Spec.TerminationGracePeriodSeconds).To(Equal(userDefinedValue))
			})
		})
	})

	Context("preference requirements check", func() {
		DescribeTable("should pass when sufficient resources are provided", func(instancetypeSpec *instancetypev1beta1.VirtualMachineInstancetypeSpec, preferenceSpec *instancetypev1beta1.VirtualMachinePreferenceSpec, vmiSpec *v1.VirtualMachineInstanceSpec) {
			conflict, err := instancetypeMethods.CheckPreferenceRequirements(instancetypeSpec, preferenceSpec, vmiSpec)
			Expect(conflict).ToNot(HaveOccurred())
			Expect(err).ToNot(HaveOccurred())
		},
			Entry("by an instance type for vCPUs",
				&instancetypev1beta1.VirtualMachineInstancetypeSpec{
					CPU: instancetypev1beta1.CPUInstancetype{
						Guest: uint32(2),
					},
				},
				&instancetypev1beta1.VirtualMachinePreferenceSpec{
					Requirements: &instancetypev1beta1.PreferenceRequirements{
						CPU: &instancetypev1beta1.CPUPreferenceRequirement{
							Guest: uint32(2),
						},
					},
				},
				&v1.VirtualMachineInstanceSpec{},
			),
			Entry("by an instance type for Memory",
				&instancetypev1beta1.VirtualMachineInstancetypeSpec{
					Memory: instancetypev1beta1.MemoryInstancetype{
						Guest: resource.MustParse("1Gi"),
					},
				},
				&instancetypev1beta1.VirtualMachinePreferenceSpec{
					Requirements: &instancetypev1beta1.PreferenceRequirements{
						Memory: &instancetypev1beta1.MemoryPreferenceRequirement{
							Guest: resource.MustParse("1Gi"),
						},
					},
				},
				&v1.VirtualMachineInstanceSpec{},
			),
			Entry("by a VM for vCPUs using PreferSockets (default)",
				nil,
				&instancetypev1beta1.VirtualMachinePreferenceSpec{
					CPU: &instancetypev1beta1.CPUPreferences{
						PreferredCPUTopology: pointer.P(instancetypev1beta1.Sockets),
					},
					Requirements: &instancetypev1beta1.PreferenceRequirements{
						CPU: &instancetypev1beta1.CPUPreferenceRequirement{
							Guest: uint32(2),
						},
					},
				},
				&v1.VirtualMachineInstanceSpec{
					Domain: v1.DomainSpec{
						CPU: &v1.CPU{
							Sockets: uint32(2),
						},
					},
				},
			),
			Entry("by a VM for vCPUs using PreferCores",
				nil,
				&instancetypev1beta1.VirtualMachinePreferenceSpec{
					CPU: &instancetypev1beta1.CPUPreferences{
						PreferredCPUTopology: pointer.P(instancetypev1beta1.Cores),
					},
					Requirements: &instancetypev1beta1.PreferenceRequirements{
						CPU: &instancetypev1beta1.CPUPreferenceRequirement{
							Guest: uint32(2),
						},
					},
				},
				&v1.VirtualMachineInstanceSpec{
					Domain: v1.DomainSpec{
						CPU: &v1.CPU{
							Cores: uint32(2),
						},
					},
				},
			),
			Entry("by a VM for vCPUs using PreferThreads",
				nil,
				&instancetypev1beta1.VirtualMachinePreferenceSpec{
					CPU: &instancetypev1beta1.CPUPreferences{
						PreferredCPUTopology: pointer.P(instancetypev1beta1.Threads),
					},
					Requirements: &instancetypev1beta1.PreferenceRequirements{
						CPU: &instancetypev1beta1.CPUPreferenceRequirement{
							Guest: uint32(2),
						},
					},
				},
				&v1.VirtualMachineInstanceSpec{
					Domain: v1.DomainSpec{
						CPU: &v1.CPU{
							Threads: uint32(2),
						},
					},
				},
			),
			Entry("by a VM for vCPUs using PreferSpread by default across SocketsCores",
				nil,
				&instancetypev1beta1.VirtualMachinePreferenceSpec{
					CPU: &instancetypev1beta1.CPUPreferences{
						PreferredCPUTopology: pointer.P(instancetypev1beta1.Spread),
					},
					Requirements: &instancetypev1beta1.PreferenceRequirements{
						CPU: &instancetypev1beta1.CPUPreferenceRequirement{
							Guest: uint32(6),
						},
					},
				},
				&v1.VirtualMachineInstanceSpec{
					Domain: v1.DomainSpec{
						CPU: &v1.CPU{
							Cores:   uint32(2),
							Sockets: uint32(3),
						},
					},
				},
			),
			Entry("by a VM for vCPUs using PreferSpread across CoresThreads",
				nil,
				&instancetypev1beta1.VirtualMachinePreferenceSpec{
					CPU: &instancetypev1beta1.CPUPreferences{
						PreferredCPUTopology: pointer.P(instancetypev1beta1.Spread),
						SpreadOptions: &instancetypev1beta1.SpreadOptions{
							Across: pointer.P(instancetypev1beta1.SpreadAcrossCoresThreads),
						},
					},
					Requirements: &instancetypev1beta1.PreferenceRequirements{
						CPU: &instancetypev1beta1.CPUPreferenceRequirement{
							Guest: uint32(6),
						},
					},
				},
				&v1.VirtualMachineInstanceSpec{
					Domain: v1.DomainSpec{
						CPU: &v1.CPU{
							Threads: uint32(2),
							Cores:   uint32(3),
						},
					},
				},
			),
			Entry("by a VM for vCPUs using PreferSpread across SocketsCoresThreads",
				nil,
				&instancetypev1beta1.VirtualMachinePreferenceSpec{
					CPU: &instancetypev1beta1.CPUPreferences{
						PreferredCPUTopology: pointer.P(instancetypev1beta1.Spread),
						SpreadOptions: &instancetypev1beta1.SpreadOptions{
							Across: pointer.P(instancetypev1beta1.SpreadAcrossSocketsCoresThreads),
						},
					},
					Requirements: &instancetypev1beta1.PreferenceRequirements{
						CPU: &instancetypev1beta1.CPUPreferenceRequirement{
							Guest: uint32(8),
						},
					},
				},
				&v1.VirtualMachineInstanceSpec{
					Domain: v1.DomainSpec{
						CPU: &v1.CPU{
							Threads: uint32(2),
							Cores:   uint32(2),
							Sockets: uint32(2),
						},
					},
				},
			),
			Entry("by a VM for vCPUs using PreferAny",
				nil,
				&instancetypev1beta1.VirtualMachinePreferenceSpec{
					CPU: &instancetypev1beta1.CPUPreferences{
						PreferredCPUTopology: pointer.P(instancetypev1beta1.Any),
					},
					Requirements: &instancetypev1beta1.PreferenceRequirements{
						CPU: &instancetypev1beta1.CPUPreferenceRequirement{
							Guest: uint32(4),
						},
					},
				},
				&v1.VirtualMachineInstanceSpec{
					Domain: v1.DomainSpec{
						CPU: &v1.CPU{
							Cores:   uint32(2),
							Sockets: uint32(2),
							Threads: uint32(1),
						},
					},
				},
			),
			Entry("by a VM for Memory",
				nil,
				&instancetypev1beta1.VirtualMachinePreferenceSpec{
					Requirements: &instancetypev1beta1.PreferenceRequirements{
						Memory: &instancetypev1beta1.MemoryPreferenceRequirement{
							Guest: resource.MustParse("1Gi"),
						},
					},
				},
				&v1.VirtualMachineInstanceSpec{
					Domain: v1.DomainSpec{
						Memory: &v1.Memory{
							Guest: resource.NewQuantity(1024*1024*1024, resource.BinarySI),
						},
					},
				},
			),
		)

		DescribeTable("should be rejected when insufficient resources are provided", func(instancetypeSpec *instancetypev1beta1.VirtualMachineInstancetypeSpec, preferenceSpec *instancetypev1beta1.VirtualMachinePreferenceSpec, vmiSpec *v1.VirtualMachineInstanceSpec, expectedConflict conflict.Conflicts, errSubString string) {
			conflicts, err := instancetypeMethods.CheckPreferenceRequirements(instancetypeSpec, preferenceSpec, vmiSpec)
			Expect(conflicts).To(Equal(expectedConflict))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(errSubString))
		},
			Entry("by an instance type for vCPUs",
				&instancetypev1beta1.VirtualMachineInstancetypeSpec{
					CPU: instancetypev1beta1.CPUInstancetype{
						Guest: uint32(1),
					},
				},
				&instancetypev1beta1.VirtualMachinePreferenceSpec{
					Requirements: &instancetypev1beta1.PreferenceRequirements{
						CPU: &instancetypev1beta1.CPUPreferenceRequirement{
							Guest: uint32(2),
						},
					},
				},
				nil,
				conflict.Conflicts{conflict.New("spec", "instancetype")},
				fmt.Sprintf(requirements.InsufficientInstanceTypeCPUResourcesErrorFmt, uint32(1), uint32(2)),
			),
			Entry("by an instance type for Memory",
				&instancetypev1beta1.VirtualMachineInstancetypeSpec{
					Memory: instancetypev1beta1.MemoryInstancetype{
						Guest: resource.MustParse("1Gi"),
					},
				},
				&instancetypev1beta1.VirtualMachinePreferenceSpec{
					Requirements: &instancetypev1beta1.PreferenceRequirements{
						Memory: &instancetypev1beta1.MemoryPreferenceRequirement{
							Guest: resource.MustParse("2Gi"),
						},
					},
				},
				nil,
				conflict.Conflicts{conflict.New("spec", "instancetype")},
				fmt.Sprintf(requirements.InsufficientInstanceTypeMemoryResourcesErrorFmt, "1Gi", "2Gi"),
			),
			Entry("by a VM for vCPUs using PreferSockets (default)",
				nil,
				&instancetypev1beta1.VirtualMachinePreferenceSpec{
					CPU: &instancetypev1beta1.CPUPreferences{
						PreferredCPUTopology: pointer.P(instancetypev1beta1.Sockets),
					},
					Requirements: &instancetypev1beta1.PreferenceRequirements{
						CPU: &instancetypev1beta1.CPUPreferenceRequirement{
							Guest: uint32(2),
						},
					},
				},
				&v1.VirtualMachineInstanceSpec{
					Domain: v1.DomainSpec{
						CPU: &v1.CPU{
							Sockets: uint32(1),
						},
					},
				},
				conflict.Conflicts{conflict.New("spec", "template", "spec", "domain", "cpu", "sockets")},
				fmt.Sprintf(requirements.InsufficientVMCPUResourcesErrorFmt, uint32(1), uint32(2), "sockets"),
			),
			Entry("by a VM for vCPUs using PreferCores",
				nil,
				&instancetypev1beta1.VirtualMachinePreferenceSpec{
					CPU: &instancetypev1beta1.CPUPreferences{
						PreferredCPUTopology: pointer.P(instancetypev1beta1.Cores),
					},
					Requirements: &instancetypev1beta1.PreferenceRequirements{
						CPU: &instancetypev1beta1.CPUPreferenceRequirement{
							Guest: uint32(2),
						},
					},
				},
				&v1.VirtualMachineInstanceSpec{
					Domain: v1.DomainSpec{
						CPU: &v1.CPU{
							Cores: uint32(1),
						},
					},
				},
				conflict.Conflicts{conflict.New("spec", "template", "spec", "domain", "cpu", "cores")},
				fmt.Sprintf(requirements.InsufficientVMCPUResourcesErrorFmt, uint32(1), uint32(2), "cores"),
			),
			Entry("by a VM for vCPUs using PreferThreads",
				nil,
				&instancetypev1beta1.VirtualMachinePreferenceSpec{
					CPU: &instancetypev1beta1.CPUPreferences{
						PreferredCPUTopology: pointer.P(instancetypev1beta1.Threads),
					},
					Requirements: &instancetypev1beta1.PreferenceRequirements{
						CPU: &instancetypev1beta1.CPUPreferenceRequirement{
							Guest: uint32(2),
						},
					},
				},
				&v1.VirtualMachineInstanceSpec{
					Domain: v1.DomainSpec{
						CPU: &v1.CPU{
							Threads: uint32(1),
						},
					},
				},
				conflict.Conflicts{conflict.New("spec", "template", "spec", "domain", "cpu", "threads")},
				fmt.Sprintf(requirements.InsufficientVMCPUResourcesErrorFmt, uint32(1), uint32(2), "threads"),
			),
			Entry("by a VM for vCPUs using PreferSpread by default across SocketsCores",
				nil,
				&instancetypev1beta1.VirtualMachinePreferenceSpec{
					CPU: &instancetypev1beta1.CPUPreferences{
						PreferredCPUTopology: pointer.P(instancetypev1beta1.Spread),
					},
					Requirements: &instancetypev1beta1.PreferenceRequirements{
						CPU: &instancetypev1beta1.CPUPreferenceRequirement{
							Guest: uint32(4),
						},
					},
				},
				&v1.VirtualMachineInstanceSpec{
					Domain: v1.DomainSpec{
						CPU: &v1.CPU{
							Cores:   uint32(1),
							Sockets: uint32(1),
						},
					},
				},
				conflict.Conflicts{conflict.New("spec", "template", "spec", "domain", "cpu", "sockets"), conflict.New("spec", "template", "spec", "domain", "cpu", "cores")},
				fmt.Sprintf(requirements.InsufficientVMCPUResourcesErrorFmt, uint32(1), uint32(4), instancetypev1beta1.SpreadAcrossSocketsCores),
			),
			Entry("by a VM for vCPUs using PreferSpread across CoresThreads",
				nil,
				&instancetypev1beta1.VirtualMachinePreferenceSpec{
					CPU: &instancetypev1beta1.CPUPreferences{
						SpreadOptions: &instancetypev1beta1.SpreadOptions{
							Across: pointer.P(instancetypev1beta1.SpreadAcrossCoresThreads),
						},
						PreferredCPUTopology: pointer.P(instancetypev1beta1.Spread),
					},
					Requirements: &instancetypev1beta1.PreferenceRequirements{
						CPU: &instancetypev1beta1.CPUPreferenceRequirement{
							Guest: uint32(4),
						},
					},
				},
				&v1.VirtualMachineInstanceSpec{
					Domain: v1.DomainSpec{
						CPU: &v1.CPU{
							Cores:   uint32(1),
							Threads: uint32(1),
						},
					},
				},
				conflict.Conflicts{conflict.New("spec", "template", "spec", "domain", "cpu", "cores"), conflict.New("spec", "template", "spec", "domain", "cpu", "threads")},
				fmt.Sprintf(requirements.InsufficientVMCPUResourcesErrorFmt, uint32(1), uint32(4), instancetypev1beta1.SpreadAcrossCoresThreads),
			),
			Entry("by a VM for vCPUs using PreferSpread across SocketsCoresThreads",
				nil,
				&instancetypev1beta1.VirtualMachinePreferenceSpec{
					CPU: &instancetypev1beta1.CPUPreferences{
						SpreadOptions: &instancetypev1beta1.SpreadOptions{
							Across: pointer.P(instancetypev1beta1.SpreadAcrossSocketsCoresThreads),
						},
						PreferredCPUTopology: pointer.P(instancetypev1beta1.Spread),
					},
					Requirements: &instancetypev1beta1.PreferenceRequirements{
						CPU: &instancetypev1beta1.CPUPreferenceRequirement{
							Guest: uint32(4),
						},
					},
				},
				&v1.VirtualMachineInstanceSpec{
					Domain: v1.DomainSpec{
						CPU: &v1.CPU{
							Sockets: uint32(1),
							Cores:   uint32(1),
							Threads: uint32(1),
						},
					},
				},
				conflict.Conflicts{conflict.New("spec", "template", "spec", "domain", "cpu", "sockets"), conflict.New("spec", "template", "spec", "domain", "cpu", "cores"), conflict.New("spec", "template", "spec", "domain", "cpu", "threads")},
				fmt.Sprintf(requirements.InsufficientVMCPUResourcesErrorFmt, uint32(1), uint32(4), instancetypev1beta1.SpreadAcrossSocketsCoresThreads),
			),
			Entry("by a VM for vCPUs using PreferAny",
				nil,
				&instancetypev1beta1.VirtualMachinePreferenceSpec{
					CPU: &instancetypev1beta1.CPUPreferences{
						PreferredCPUTopology: pointer.P(instancetypev1beta1.Any),
					},
					Requirements: &instancetypev1beta1.PreferenceRequirements{
						CPU: &instancetypev1beta1.CPUPreferenceRequirement{
							Guest: uint32(4),
						},
					},
				},
				&v1.VirtualMachineInstanceSpec{
					Domain: v1.DomainSpec{
						CPU: &v1.CPU{
							Sockets: uint32(2),
							Cores:   uint32(1),
							Threads: uint32(1),
						},
					},
				},
				conflict.Conflicts{
					conflict.New("spec", "template", "spec", "domain", "cpu", "cores"),
					conflict.New("spec", "template", "spec", "domain", "cpu", "sockets"),
					conflict.New("spec", "template", "spec", "domain", "cpu", "threads"),
				},
				fmt.Sprintf(requirements.InsufficientVMCPUResourcesErrorFmt, uint32(2), uint32(4), "cores, sockets and threads"),
			),
			Entry("by a VM for Memory",
				nil,
				&instancetypev1beta1.VirtualMachinePreferenceSpec{
					Requirements: &instancetypev1beta1.PreferenceRequirements{
						Memory: &instancetypev1beta1.MemoryPreferenceRequirement{
							Guest: resource.MustParse("2Gi"),
						},
					},
				},
				&v1.VirtualMachineInstanceSpec{
					Domain: v1.DomainSpec{
						Memory: &v1.Memory{
							Guest: resource.NewQuantity(1024*1024*1024, resource.BinarySI),
						},
					},
				},
				conflict.Conflicts{conflict.New("spec", "template", "spec", "domain", "memory")},
				fmt.Sprintf(requirements.InsufficientVMMemoryResourcesErrorFmt, "1Gi", "2Gi"),
			))
	})
})
