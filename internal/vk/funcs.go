//go:build (windows || linux || darwin) && (amd64 || arm64)

package vk

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// Global commands, resolved from the loader library.
var (
	vkGetInstanceProcAddr                  func(instance uintptr, name string) uintptr
	vkCreateInstance                       func(info *instanceCreateInfo, alloc unsafe.Pointer, instance *uintptr) Result
	vkEnumerateInstanceExtensionProperties func(layer *byte, count *uint32, props *extensionProperties) Result
)

// Instance and device commands. Device commands are resolved through
// vkGetInstanceProcAddr too: the loader trampolines work for any device,
// and the per-call dispatch overhead is irrelevant here.
var (
	vkDestroyInstance                        func(instance uintptr, alloc unsafe.Pointer)
	vkEnumeratePhysicalDevices               func(instance uintptr, count *uint32, devices *uintptr) Result
	vkGetPhysicalDeviceProperties            func(pd uintptr, props *physicalDeviceProperties)
	vkGetPhysicalDeviceFeatures              func(pd uintptr, features *physicalDeviceFeatures)
	vkGetPhysicalDeviceQueueFamilyProperties func(pd uintptr, count *uint32, props *queueFamilyProperties)
	vkGetPhysicalDeviceMemoryProperties      func(pd uintptr, props *physicalDeviceMemoryProperties)
	vkEnumerateDeviceExtensionProperties     func(pd uintptr, layer *byte, count *uint32, props *extensionProperties) Result
	vkCreateDevice                           func(pd uintptr, info *deviceCreateInfo, alloc unsafe.Pointer, device *uintptr) Result

	vkDestroyDevice               func(device uintptr, alloc unsafe.Pointer)
	vkGetDeviceQueue              func(device uintptr, family, index uint32, queue *uintptr)
	vkDeviceWaitIdle              func(device uintptr) Result
	vkCreateBuffer                func(device uintptr, info *bufferCreateInfo, alloc unsafe.Pointer, buffer *uint64) Result
	vkDestroyBuffer               func(device uintptr, buffer uint64, alloc unsafe.Pointer)
	vkGetBufferMemoryRequirements func(device uintptr, buffer uint64, req *memoryRequirements)
	vkAllocateMemory              func(device uintptr, info *memoryAllocateInfo, alloc unsafe.Pointer, memory *uint64) Result
	vkFreeMemory                  func(device uintptr, memory uint64, alloc unsafe.Pointer)
	vkBindBufferMemory            func(device uintptr, buffer, memory, offset uint64) Result
	vkMapMemory                   func(device uintptr, memory, offset, size uint64, flags uint32, data *unsafe.Pointer) Result
	vkUnmapMemory                 func(device uintptr, memory uint64)
	vkCreateShaderModule          func(device uintptr, info *shaderModuleCreateInfo, alloc unsafe.Pointer, module *uint64) Result
	vkDestroyShaderModule         func(device uintptr, module uint64, alloc unsafe.Pointer)
	vkCreateDescriptorSetLayout   func(device uintptr, info *descriptorSetLayoutCreateInfo, alloc unsafe.Pointer, layout *uint64) Result
	vkDestroyDescriptorSetLayout  func(device uintptr, layout uint64, alloc unsafe.Pointer)
	vkCreatePipelineLayout        func(device uintptr, info *pipelineLayoutCreateInfo, alloc unsafe.Pointer, layout *uint64) Result
	vkDestroyPipelineLayout       func(device uintptr, layout uint64, alloc unsafe.Pointer)
	vkCreateComputePipelines      func(device uintptr, cache uint64, count uint32, infos *computePipelineCreateInfo, alloc unsafe.Pointer, pipelines *uint64) Result
	vkDestroyPipeline             func(device uintptr, pipeline uint64, alloc unsafe.Pointer)
	vkCreateDescriptorPool        func(device uintptr, info *descriptorPoolCreateInfo, alloc unsafe.Pointer, pool *uint64) Result
	vkDestroyDescriptorPool       func(device uintptr, pool uint64, alloc unsafe.Pointer)
	vkAllocateDescriptorSets      func(device uintptr, info *descriptorSetAllocateInfo, sets *uint64) Result
	vkUpdateDescriptorSets        func(device uintptr, writeCount uint32, writes *writeDescriptorSet, copyCount uint32, copies unsafe.Pointer)
	vkCreateCommandPool           func(device uintptr, info *commandPoolCreateInfo, alloc unsafe.Pointer, pool *uint64) Result
	vkDestroyCommandPool          func(device uintptr, pool uint64, alloc unsafe.Pointer)
	vkAllocateCommandBuffers      func(device uintptr, info *commandBufferAllocateInfo, buffers *uintptr) Result
	vkBeginCommandBuffer          func(cb uintptr, info *commandBufferBeginInfo) Result
	vkEndCommandBuffer            func(cb uintptr) Result
	vkResetCommandBuffer          func(cb uintptr, flags uint32) Result
	vkCmdBindPipeline             func(cb uintptr, bindPoint uint32, pipeline uint64)
	vkCmdBindDescriptorSets       func(cb uintptr, bindPoint uint32, layout uint64, firstSet, count uint32, sets *uint64, dynamicCount uint32, dynamicOffsets *uint32)
	vkCmdPushConstants            func(cb uintptr, layout uint64, stages, offset, size uint32, values unsafe.Pointer)
	vkCmdDispatch                 func(cb uintptr, x, y, z uint32)
	vkCmdPipelineBarrier          func(cb uintptr, srcStages, dstStages, dependencyFlags, memoryBarrierCount uint32, memoryBarriers *memoryBarrier, bufferBarrierCount uint32, bufferBarriers unsafe.Pointer, imageBarrierCount uint32, imageBarriers unsafe.Pointer)
	vkCreateFence                 func(device uintptr, info *fenceCreateInfo, alloc unsafe.Pointer, fence *uint64) Result
	vkDestroyFence                func(device uintptr, fence uint64, alloc unsafe.Pointer)
	vkResetFences                 func(device uintptr, count uint32, fences *uint64) Result
	vkWaitForFences               func(device uintptr, count uint32, fences *uint64, waitAll uint32, timeout uint64) Result
	vkQueueSubmit                 func(queue uintptr, count uint32, submits *submitInfo, fence uint64) Result
)

var (
	loadOnce sync.Once
	loadErr  error
)

// loadGlobal opens the Vulkan loader and resolves the global commands.
func loadGlobal() error {
	loadOnce.Do(func() {
		lib, err := openLibrary()
		if err != nil {
			loadErr = fmt.Errorf("Vulkan loader not found (is a Vulkan driver installed?): %w", err)
			return
		}
		sym, err := symbol(lib, "vkGetInstanceProcAddr")
		if err != nil {
			loadErr = err
			return
		}
		purego.RegisterFunc(&vkGetInstanceProcAddr, sym)
		loadErr = resolve(0, map[string]any{
			"vkCreateInstance":                       &vkCreateInstance,
			"vkEnumerateInstanceExtensionProperties": &vkEnumerateInstanceExtensionProperties,
		})
		// Absent from Vulkan 1.0 loaders.
		resolveOptional(0, map[string]any{"vkEnumerateInstanceVersion": &vkEnumerateInstanceVersion})
	})
	return loadErr
}

// loadInstance resolves every instance and device command used by this package.
func loadInstance(instance uintptr) error {
	resolveOptional(instance, map[string]any{
		"vkGetPhysicalDeviceFeatures2":         &vkGetPhysicalDeviceFeatures2,
		"vkGetPhysicalDeviceProperties2":       &vkGetPhysicalDeviceProperties2,
		"vkGetPipelineExecutablePropertiesKHR": &vkGetPipelineExecutablePropertiesKHR,
		"vkGetPipelineExecutableStatisticsKHR": &vkGetPipelineExecutableStatisticsKHR,
	})
	return resolve(instance, map[string]any{
		"vkDestroyInstance":                        &vkDestroyInstance,
		"vkEnumeratePhysicalDevices":               &vkEnumeratePhysicalDevices,
		"vkGetPhysicalDeviceProperties":            &vkGetPhysicalDeviceProperties,
		"vkGetPhysicalDeviceFeatures":              &vkGetPhysicalDeviceFeatures,
		"vkGetPhysicalDeviceQueueFamilyProperties": &vkGetPhysicalDeviceQueueFamilyProperties,
		"vkGetPhysicalDeviceMemoryProperties":      &vkGetPhysicalDeviceMemoryProperties,
		"vkEnumerateDeviceExtensionProperties":     &vkEnumerateDeviceExtensionProperties,
		"vkCreateDevice":                           &vkCreateDevice,
		"vkDestroyDevice":                          &vkDestroyDevice,
		"vkGetDeviceQueue":                         &vkGetDeviceQueue,
		"vkDeviceWaitIdle":                         &vkDeviceWaitIdle,
		"vkCreateBuffer":                           &vkCreateBuffer,
		"vkDestroyBuffer":                          &vkDestroyBuffer,
		"vkGetBufferMemoryRequirements":            &vkGetBufferMemoryRequirements,
		"vkAllocateMemory":                         &vkAllocateMemory,
		"vkFreeMemory":                             &vkFreeMemory,
		"vkBindBufferMemory":                       &vkBindBufferMemory,
		"vkMapMemory":                              &vkMapMemory,
		"vkUnmapMemory":                            &vkUnmapMemory,
		"vkCreateShaderModule":                     &vkCreateShaderModule,
		"vkDestroyShaderModule":                    &vkDestroyShaderModule,
		"vkCreateDescriptorSetLayout":              &vkCreateDescriptorSetLayout,
		"vkDestroyDescriptorSetLayout":             &vkDestroyDescriptorSetLayout,
		"vkCreatePipelineLayout":                   &vkCreatePipelineLayout,
		"vkDestroyPipelineLayout":                  &vkDestroyPipelineLayout,
		"vkCreateComputePipelines":                 &vkCreateComputePipelines,
		"vkDestroyPipeline":                        &vkDestroyPipeline,
		"vkCreateDescriptorPool":                   &vkCreateDescriptorPool,
		"vkDestroyDescriptorPool":                  &vkDestroyDescriptorPool,
		"vkAllocateDescriptorSets":                 &vkAllocateDescriptorSets,
		"vkUpdateDescriptorSets":                   &vkUpdateDescriptorSets,
		"vkCreateCommandPool":                      &vkCreateCommandPool,
		"vkDestroyCommandPool":                     &vkDestroyCommandPool,
		"vkAllocateCommandBuffers":                 &vkAllocateCommandBuffers,
		"vkBeginCommandBuffer":                     &vkBeginCommandBuffer,
		"vkEndCommandBuffer":                       &vkEndCommandBuffer,
		"vkResetCommandBuffer":                     &vkResetCommandBuffer,
		"vkCmdBindPipeline":                        &vkCmdBindPipeline,
		"vkCmdBindDescriptorSets":                  &vkCmdBindDescriptorSets,
		"vkCmdPushConstants":                       &vkCmdPushConstants,
		"vkCmdDispatch":                            &vkCmdDispatch,
		"vkCmdPipelineBarrier":                     &vkCmdPipelineBarrier,
		"vkCreateFence":                            &vkCreateFence,
		"vkDestroyFence":                           &vkDestroyFence,
		"vkResetFences":                            &vkResetFences,
		"vkWaitForFences":                          &vkWaitForFences,
		"vkQueueSubmit":                            &vkQueueSubmit,
	})
}

func resolve(instance uintptr, fns map[string]any) error {
	for name, fptr := range fns {
		addr := vkGetInstanceProcAddr(instance, name+"\x00")
		if addr == 0 {
			return fmt.Errorf("Vulkan loader does not provide %s", name)
		}
		purego.RegisterFunc(fptr, addr)
	}
	return nil
}
