//go:build (windows || linux || darwin) && (amd64 || arm64)

package vk

import (
	"testing"
	"unsafe"
)

// Expected values were measured with sizeof/offsetof against vulkan_core.h
// (Vulkan SDK 1.4.350, 64-bit). A mismatch means a struct no longer matches
// the C ABI and the driver would read or write the wrong fields.
func TestStructLayout(t *testing.T) {
	sizes := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"VkApplicationInfo", unsafe.Sizeof(applicationInfo{}), 48},
		{"VkInstanceCreateInfo", unsafe.Sizeof(instanceCreateInfo{}), 64},
		{"VkDeviceQueueCreateInfo", unsafe.Sizeof(deviceQueueCreateInfo{}), 40},
		{"VkDeviceCreateInfo", unsafe.Sizeof(deviceCreateInfo{}), 72},
		{"VkQueueFamilyProperties", unsafe.Sizeof(queueFamilyProperties{}), 24},
		{"VkPhysicalDeviceProperties", unsafe.Sizeof(physicalDeviceProperties{}), 824},
		{"VkPhysicalDeviceFeatures", unsafe.Sizeof(physicalDeviceFeatures{}), 220},
		{"VkPhysicalDeviceMemoryProperties", unsafe.Sizeof(physicalDeviceMemoryProperties{}), 520},
		{"VkBufferCreateInfo", unsafe.Sizeof(bufferCreateInfo{}), 56},
		{"VkMemoryRequirements", unsafe.Sizeof(memoryRequirements{}), 24},
		{"VkMemoryAllocateInfo", unsafe.Sizeof(memoryAllocateInfo{}), 32},
		{"VkShaderModuleCreateInfo", unsafe.Sizeof(shaderModuleCreateInfo{}), 40},
		{"VkDescriptorSetLayoutBinding", unsafe.Sizeof(descriptorSetLayoutBinding{}), 24},
		{"VkDescriptorSetLayoutCreateInfo", unsafe.Sizeof(descriptorSetLayoutCreateInfo{}), 32},
		{"VkPushConstantRange", unsafe.Sizeof(pushConstantRange{}), 12},
		{"VkPipelineLayoutCreateInfo", unsafe.Sizeof(pipelineLayoutCreateInfo{}), 48},
		{"VkSpecializationMapEntry", unsafe.Sizeof(specializationMapEntry{}), 16},
		{"VkSpecializationInfo", unsafe.Sizeof(specializationInfo{}), 32},
		{"VkPipelineShaderStageCreateInfo", unsafe.Sizeof(pipelineShaderStageCreateInfo{}), 48},
		{"VkComputePipelineCreateInfo", unsafe.Sizeof(computePipelineCreateInfo{}), 96},
		{"VkDescriptorPoolSize", unsafe.Sizeof(descriptorPoolSize{}), 8},
		{"VkDescriptorPoolCreateInfo", unsafe.Sizeof(descriptorPoolCreateInfo{}), 40},
		{"VkDescriptorSetAllocateInfo", unsafe.Sizeof(descriptorSetAllocateInfo{}), 40},
		{"VkDescriptorBufferInfo", unsafe.Sizeof(descriptorBufferInfo{}), 24},
		{"VkWriteDescriptorSet", unsafe.Sizeof(writeDescriptorSet{}), 64},
		{"VkCommandPoolCreateInfo", unsafe.Sizeof(commandPoolCreateInfo{}), 24},
		{"VkCommandBufferAllocateInfo", unsafe.Sizeof(commandBufferAllocateInfo{}), 32},
		{"VkCommandBufferBeginInfo", unsafe.Sizeof(commandBufferBeginInfo{}), 32},
		{"VkMemoryBarrier", unsafe.Sizeof(memoryBarrier{}), 24},
		{"VkFenceCreateInfo", unsafe.Sizeof(fenceCreateInfo{}), 24},
		{"VkSubmitInfo", unsafe.Sizeof(submitInfo{}), 72},
		{"VkExtensionProperties", unsafe.Sizeof(extensionProperties{}), 260},
	}
	for _, s := range sizes {
		if s.got != s.want {
			t.Errorf("sizeof(%s) = %d, want %d", s.name, s.got, s.want)
		}
	}

	offsets := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"VkApplicationInfo.apiVersion", unsafe.Offsetof(applicationInfo{}.apiVersion), 44},
		{"VkInstanceCreateInfo.ppEnabledExtensionNames", unsafe.Offsetof(instanceCreateInfo{}.ppEnabledExtensionNames), 56},
		{"VkDeviceQueueCreateInfo.pQueuePriorities", unsafe.Offsetof(deviceQueueCreateInfo{}.pQueuePriorities), 32},
		{"VkDeviceCreateInfo.pEnabledFeatures", unsafe.Offsetof(deviceCreateInfo{}.pEnabledFeatures), 64},
		{"VkPhysicalDeviceProperties.deviceName", unsafe.Offsetof(physicalDeviceProperties{}.deviceName), 20},
		{"VkPhysicalDeviceFeatures.shaderInt64", featureShaderInt64 * 4, 160},
		{"VkPhysicalDeviceMemoryProperties.memoryHeapCount", unsafe.Offsetof(physicalDeviceMemoryProperties{}.memoryHeapCount), 260},
		{"VkPhysicalDeviceMemoryProperties.memoryHeaps", unsafe.Offsetof(physicalDeviceMemoryProperties{}.memoryHeaps), 264},
		{"VkBufferCreateInfo.pQueueFamilyIndices", unsafe.Offsetof(bufferCreateInfo{}.pQueueFamilyIndices), 48},
		{"VkShaderModuleCreateInfo.pCode", unsafe.Offsetof(shaderModuleCreateInfo{}.pCode), 32},
		{"VkPipelineLayoutCreateInfo.pPushConstantRanges", unsafe.Offsetof(pipelineLayoutCreateInfo{}.pPushConstantRanges), 40},
		{"VkSpecializationMapEntry.size", unsafe.Offsetof(specializationMapEntry{}.size), 8},
		{"VkSpecializationInfo.pMapEntries", unsafe.Offsetof(specializationInfo{}.pMapEntries), 8},
		{"VkSpecializationInfo.dataSize", unsafe.Offsetof(specializationInfo{}.dataSize), 16},
		{"VkSpecializationInfo.pData", unsafe.Offsetof(specializationInfo{}.pData), 24},
		{"VkPipelineShaderStageCreateInfo.pName", unsafe.Offsetof(pipelineShaderStageCreateInfo{}.pName), 32},
		{"VkPipelineShaderStageCreateInfo.pSpecializationInfo", unsafe.Offsetof(pipelineShaderStageCreateInfo{}.pSpecializationInfo), 40},
		{"VkComputePipelineCreateInfo.stage", unsafe.Offsetof(computePipelineCreateInfo{}.stage), 24},
		{"VkComputePipelineCreateInfo.layout", unsafe.Offsetof(computePipelineCreateInfo{}.layout), 72},
		{"VkComputePipelineCreateInfo.basePipelineIndex", unsafe.Offsetof(computePipelineCreateInfo{}.basePipelineIndex), 88},
		{"VkDescriptorPoolCreateInfo.pPoolSizes", unsafe.Offsetof(descriptorPoolCreateInfo{}.pPoolSizes), 32},
		{"VkDescriptorSetAllocateInfo.pSetLayouts", unsafe.Offsetof(descriptorSetAllocateInfo{}.pSetLayouts), 32},
		{"VkWriteDescriptorSet.descriptorType", unsafe.Offsetof(writeDescriptorSet{}.descriptorType), 36},
		{"VkWriteDescriptorSet.pBufferInfo", unsafe.Offsetof(writeDescriptorSet{}.pBufferInfo), 48},
		{"VkCommandBufferAllocateInfo.commandBufferCount", unsafe.Offsetof(commandBufferAllocateInfo{}.commandBufferCount), 28},
		{"VkSubmitInfo.pSignalSemaphores", unsafe.Offsetof(submitInfo{}.pSignalSemaphores), 64},
	}
	for _, o := range offsets {
		if o.got != o.want {
			t.Errorf("offsetof(%s) = %d, want %d", o.name, o.got, o.want)
		}
	}
}
