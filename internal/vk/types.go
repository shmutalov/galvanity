//go:build (windows || linux || darwin) && (amd64 || arm64)

package vk

import "unsafe"

// Only the subset of the Vulkan 1.0 API needed to run a compute shader is
// declared here. Struct layouts mirror vulkan_core.h on 64-bit targets
// (layout_test.go pins the sizes and offsets). Dispatchable handles are
// pointers (uintptr), non-dispatchable handles are uint64.

type Result int32

const (
	resultTimeout         Result = 2
	resultErrorDeviceLost Result = -4

	apiVersion10 = 1 << 22
	wholeSize    = ^uint64(0)
)

const (
	stApplicationInfo           = 0
	stInstanceCreateInfo        = 1
	stDeviceQueueCreateInfo     = 2
	stDeviceCreateInfo          = 3
	stSubmitInfo                = 4
	stMemoryAllocateInfo        = 5
	stFenceCreateInfo           = 8
	stBufferCreateInfo          = 12
	stShaderModuleCreateInfo    = 16
	stPipelineShaderStageCreate = 18
	stComputePipelineCreateInfo = 29
	stPipelineLayoutCreateInfo  = 30
	stDescriptorSetLayoutCreate = 32
	stDescriptorPoolCreateInfo  = 33
	stDescriptorSetAllocateInfo = 34
	stWriteDescriptorSet        = 35
	stCommandPoolCreateInfo     = 39
	stCommandBufferAllocateInfo = 40
	stCommandBufferBeginInfo    = 42
	stMemoryBarrier             = 46
)

const (
	queueGraphicsBit = 0x1
	queueComputeBit  = 0x2

	bufferUsageStorageBufferBit = 0x20

	memoryPropertyDeviceLocalBit  = 0x1
	memoryPropertyHostVisibleBit  = 0x2
	memoryPropertyHostCoherentBit = 0x4
	memoryPropertyHostCachedBit   = 0x8

	descriptorTypeStorageBuffer = 7
	shaderStageComputeBit       = 0x20
	pipelineBindPointCompute    = 1

	commandPoolCreateResetCommandBufferBit = 0x2
	commandBufferUsageOneTimeSubmitBit     = 0x1

	pipelineStageComputeShaderBit = 0x800
	pipelineStageHostBit          = 0x4000
	accessShaderWriteBit          = 0x40
	accessHostReadBit             = 0x2000

	instanceCreateEnumeratePortabilityBit = 0x1
)

const (
	extPortabilityEnumeration = "VK_KHR_portability_enumeration"
	extPortabilitySubset      = "VK_KHR_portability_subset"
)

type applicationInfo struct {
	sType              uint32
	pNext              unsafe.Pointer
	pApplicationName   *byte
	applicationVersion uint32
	pEngineName        *byte
	engineVersion      uint32
	apiVersion         uint32
}

type instanceCreateInfo struct {
	sType                   uint32
	pNext                   unsafe.Pointer
	flags                   uint32
	pApplicationInfo        *applicationInfo
	enabledLayerCount       uint32
	ppEnabledLayerNames     **byte
	enabledExtensionCount   uint32
	ppEnabledExtensionNames **byte
}

type extensionProperties struct {
	extensionName [256]byte
	specVersion   uint32
}

type deviceQueueCreateInfo struct {
	sType            uint32
	pNext            unsafe.Pointer
	flags            uint32
	queueFamilyIndex uint32
	queueCount       uint32
	pQueuePriorities *float32
}

type deviceCreateInfo struct {
	sType                   uint32
	pNext                   unsafe.Pointer
	flags                   uint32
	queueCreateInfoCount    uint32
	pQueueCreateInfos       *deviceQueueCreateInfo
	enabledLayerCount       uint32
	ppEnabledLayerNames     **byte
	enabledExtensionCount   uint32
	ppEnabledExtensionNames **byte
	pEnabledFeatures        *physicalDeviceFeatures
}

type queueFamilyProperties struct {
	queueFlags                  uint32
	queueCount                  uint32
	timestampValidBits          uint32
	minImageTransferGranularity [3]uint32
}

// physicalDeviceProperties declares the leading fields only; limits and
// sparseProperties are kept as opaque bytes.
type physicalDeviceProperties struct {
	apiVersion        uint32
	driverVersion     uint32
	vendorID          uint32
	deviceID          uint32
	deviceType        uint32
	deviceName        [256]byte
	pipelineCacheUUID [16]byte
	_                 [532]byte // VkPhysicalDeviceLimits + VkPhysicalDeviceSparseProperties + tail padding
}

// physicalDeviceFeatures is VkPhysicalDeviceFeatures: 55 VkBool32 fields.
type physicalDeviceFeatures [55]uint32

const featureShaderInt64 = 40

type memoryType struct {
	propertyFlags uint32
	heapIndex     uint32
}

type memoryHeap struct {
	size  uint64
	flags uint32
}

type physicalDeviceMemoryProperties struct {
	memoryTypeCount uint32
	memoryTypes     [32]memoryType
	memoryHeapCount uint32
	memoryHeaps     [16]memoryHeap
}

type bufferCreateInfo struct {
	sType                 uint32
	pNext                 unsafe.Pointer
	flags                 uint32
	size                  uint64
	usage                 uint32
	sharingMode           uint32
	queueFamilyIndexCount uint32
	pQueueFamilyIndices   *uint32
}

type memoryRequirements struct {
	size           uint64
	alignment      uint64
	memoryTypeBits uint32
}

type memoryAllocateInfo struct {
	sType           uint32
	pNext           unsafe.Pointer
	allocationSize  uint64
	memoryTypeIndex uint32
}

type shaderModuleCreateInfo struct {
	sType    uint32
	pNext    unsafe.Pointer
	flags    uint32
	codeSize uintptr
	pCode    *uint32
}

type descriptorSetLayoutBinding struct {
	binding            uint32
	descriptorType     uint32
	descriptorCount    uint32
	stageFlags         uint32
	pImmutableSamplers unsafe.Pointer
}

type descriptorSetLayoutCreateInfo struct {
	sType        uint32
	pNext        unsafe.Pointer
	flags        uint32
	bindingCount uint32
	pBindings    *descriptorSetLayoutBinding
}

type pushConstantRange struct {
	stageFlags uint32
	offset     uint32
	size       uint32
}

type pipelineLayoutCreateInfo struct {
	sType                  uint32
	pNext                  unsafe.Pointer
	flags                  uint32
	setLayoutCount         uint32
	pSetLayouts            *uint64
	pushConstantRangeCount uint32
	pPushConstantRanges    *pushConstantRange
}

type specializationMapEntry struct {
	constantID uint32
	offset     uint32
	size       uintptr
}

type specializationInfo struct {
	mapEntryCount uint32
	pMapEntries   *specializationMapEntry
	dataSize      uintptr
	pData         unsafe.Pointer
}

type pipelineShaderStageCreateInfo struct {
	sType               uint32
	pNext               unsafe.Pointer
	flags               uint32
	stage               uint32
	module              uint64
	pName               *byte
	pSpecializationInfo *specializationInfo
}

type computePipelineCreateInfo struct {
	sType              uint32
	pNext              unsafe.Pointer
	flags              uint32
	stage              pipelineShaderStageCreateInfo
	layout             uint64
	basePipelineHandle uint64
	basePipelineIndex  int32
}

type descriptorPoolSize struct {
	typ             uint32
	descriptorCount uint32
}

type descriptorPoolCreateInfo struct {
	sType         uint32
	pNext         unsafe.Pointer
	flags         uint32
	maxSets       uint32
	poolSizeCount uint32
	pPoolSizes    *descriptorPoolSize
}

type descriptorSetAllocateInfo struct {
	sType              uint32
	pNext              unsafe.Pointer
	descriptorPool     uint64
	descriptorSetCount uint32
	pSetLayouts        *uint64
}

type descriptorBufferInfo struct {
	buffer uint64
	offset uint64
	rng    uint64
}

type writeDescriptorSet struct {
	sType            uint32
	pNext            unsafe.Pointer
	dstSet           uint64
	dstBinding       uint32
	dstArrayElement  uint32
	descriptorCount  uint32
	descriptorType   uint32
	pImageInfo       unsafe.Pointer
	pBufferInfo      *descriptorBufferInfo
	pTexelBufferView unsafe.Pointer
}

type commandPoolCreateInfo struct {
	sType            uint32
	pNext            unsafe.Pointer
	flags            uint32
	queueFamilyIndex uint32
}

type commandBufferAllocateInfo struct {
	sType              uint32
	pNext              unsafe.Pointer
	commandPool        uint64
	level              uint32
	commandBufferCount uint32
}

type commandBufferBeginInfo struct {
	sType            uint32
	pNext            unsafe.Pointer
	flags            uint32
	pInheritanceInfo unsafe.Pointer
}

type memoryBarrier struct {
	sType         uint32
	pNext         unsafe.Pointer
	srcAccessMask uint32
	dstAccessMask uint32
}

type fenceCreateInfo struct {
	sType uint32
	pNext unsafe.Pointer
	flags uint32
}

type submitInfo struct {
	sType                uint32
	pNext                unsafe.Pointer
	waitSemaphoreCount   uint32
	pWaitSemaphores      *uint64
	pWaitDstStageMask    *uint32
	commandBufferCount   uint32
	pCommandBuffers      *uintptr
	signalSemaphoreCount uint32
	pSignalSemaphores    *uint64
}
