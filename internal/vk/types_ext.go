//go:build (windows || linux || darwin) && (amd64 || arm64)

package vk

import "unsafe"

// Optional Vulkan 1.1+ / extension structures used for subgroup size control
// and pipeline statistics. Layouts are pinned in layout_test.go.

const (
	apiVersion11 = 1<<22 | 1<<12
	apiVersion13 = 1<<22 | 3<<12

	stPhysicalDeviceFeatures2                       = 1000059000
	stPhysicalDeviceProperties2                     = 1000059001
	stPhysicalDeviceSubgroupProperties              = 1000094000
	stPhysicalDeviceSubgroupSizeControlProperties   = 1000225000
	stPipelineShaderStageRequiredSubgroupSizeCreate = 1000225001
	stPhysicalDeviceSubgroupSizeControlFeatures     = 1000225002
	stPhysicalDevicePipelineExecutablePropsFeatures = 1000269000
	stPipelineInfo                                  = 1000269001
	stPipelineExecutableProperties                  = 1000269002
	stPipelineExecutableInfo                        = 1000269003
	stPipelineExecutableStatistic                   = 1000269004

	pipelineCreateCaptureStatisticsBit = 0x40

	statFormatBool32  = 0
	statFormatInt64   = 1
	statFormatUint64  = 2
	statFormatFloat64 = 3

	extSubgroupSizeControl          = "VK_EXT_subgroup_size_control"
	extPipelineExecutableProperties = "VK_KHR_pipeline_executable_properties"
)

type physicalDeviceFeatures2 struct {
	sType    uint32
	pNext    unsafe.Pointer
	features physicalDeviceFeatures
}

type physicalDeviceProperties2 struct {
	sType      uint32
	pNext      unsafe.Pointer
	properties physicalDeviceProperties
}

type physicalDeviceSubgroupProperties struct {
	sType                     uint32
	pNext                     unsafe.Pointer
	subgroupSize              uint32
	supportedStages           uint32
	supportedOperations       uint32
	quadOperationsInAllStages uint32
}

type physicalDeviceSubgroupSizeControlFeatures struct {
	sType                uint32
	pNext                unsafe.Pointer
	subgroupSizeControl  uint32
	computeFullSubgroups uint32
}

type physicalDeviceSubgroupSizeControlProperties struct {
	sType                        uint32
	pNext                        unsafe.Pointer
	minSubgroupSize              uint32
	maxSubgroupSize              uint32
	maxComputeWorkgroupSubgroups uint32
	requiredSubgroupSizeStages   uint32
}

type pipelineShaderStageRequiredSubgroupSizeCreateInfo struct {
	sType                uint32
	pNext                unsafe.Pointer
	requiredSubgroupSize uint32
}

type physicalDevicePipelineExecutablePropertiesFeatures struct {
	sType                  uint32
	pNext                  unsafe.Pointer
	pipelineExecutableInfo uint32
}

type pipelineInfo struct {
	sType    uint32
	pNext    unsafe.Pointer
	pipeline uint64
}

type pipelineExecutableProperties struct {
	sType        uint32
	pNext        unsafe.Pointer
	stages       uint32
	name         [256]byte
	description  [256]byte
	subgroupSize uint32
}

type pipelineExecutableInfo struct {
	sType           uint32
	pNext           unsafe.Pointer
	pipeline        uint64
	executableIndex uint32
}

// pipelineExecutableStatistic.value is the VkPipelineExecutableStatisticValueKHR
// union; format says which member is valid.
type pipelineExecutableStatistic struct {
	sType       uint32
	pNext       unsafe.Pointer
	name        [256]byte
	description [256]byte
	format      uint32
	value       uint64
}
