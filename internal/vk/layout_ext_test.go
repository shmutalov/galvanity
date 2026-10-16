//go:build (windows || linux || darwin) && (amd64 || arm64)

package vk

import (
	"testing"
	"unsafe"
)

func TestStructLayoutExt(t *testing.T) {
	type c struct {
		name      string
		got, want uintptr
	}
	for _, s := range []c{
		{"VkPhysicalDeviceFeatures2", unsafe.Sizeof(physicalDeviceFeatures2{}), 240},
		{"VkPhysicalDeviceProperties2", unsafe.Sizeof(physicalDeviceProperties2{}), 840},
		{"VkPhysicalDeviceSubgroupProperties", unsafe.Sizeof(physicalDeviceSubgroupProperties{}), 32},
		{"VkPhysicalDeviceSubgroupSizeControlFeatures", unsafe.Sizeof(physicalDeviceSubgroupSizeControlFeatures{}), 24},
		{"VkPhysicalDeviceSubgroupSizeControlProperties", unsafe.Sizeof(physicalDeviceSubgroupSizeControlProperties{}), 32},
		{"VkPipelineShaderStageRequiredSubgroupSizeCreateInfo", unsafe.Sizeof(pipelineShaderStageRequiredSubgroupSizeCreateInfo{}), 24},
		{"VkPhysicalDevicePipelineExecutablePropertiesFeaturesKHR", unsafe.Sizeof(physicalDevicePipelineExecutablePropertiesFeatures{}), 24},
		{"VkPipelineInfoKHR", unsafe.Sizeof(pipelineInfo{}), 24},
		{"VkPipelineExecutablePropertiesKHR", unsafe.Sizeof(pipelineExecutableProperties{}), 536},
		{"VkPipelineExecutableInfoKHR", unsafe.Sizeof(pipelineExecutableInfo{}), 32},
		{"VkPipelineExecutableStatisticKHR", unsafe.Sizeof(pipelineExecutableStatistic{}), 544},

		{"VkPhysicalDeviceFeatures2.features", unsafe.Offsetof(physicalDeviceFeatures2{}.features), 16},
		{"VkPhysicalDeviceProperties2.properties", unsafe.Offsetof(physicalDeviceProperties2{}.properties), 16},
		{"VkPhysicalDeviceSubgroupProperties.subgroupSize", unsafe.Offsetof(physicalDeviceSubgroupProperties{}.subgroupSize), 16},
		{"VkPhysicalDeviceSubgroupSizeControlFeatures.subgroupSizeControl", unsafe.Offsetof(physicalDeviceSubgroupSizeControlFeatures{}.subgroupSizeControl), 16},
		{"VkPhysicalDeviceSubgroupSizeControlProperties.minSubgroupSize", unsafe.Offsetof(physicalDeviceSubgroupSizeControlProperties{}.minSubgroupSize), 16},
		{"VkPhysicalDeviceSubgroupSizeControlProperties.requiredSubgroupSizeStages", unsafe.Offsetof(physicalDeviceSubgroupSizeControlProperties{}.requiredSubgroupSizeStages), 28},
		{"VkPipelineShaderStageRequiredSubgroupSizeCreateInfo.requiredSubgroupSize", unsafe.Offsetof(pipelineShaderStageRequiredSubgroupSizeCreateInfo{}.requiredSubgroupSize), 16},
		{"VkPhysicalDevicePipelineExecutablePropertiesFeaturesKHR.pipelineExecutableInfo", unsafe.Offsetof(physicalDevicePipelineExecutablePropertiesFeatures{}.pipelineExecutableInfo), 16},
		{"VkPipelineInfoKHR.pipeline", unsafe.Offsetof(pipelineInfo{}.pipeline), 16},
		{"VkPipelineExecutablePropertiesKHR.stages", unsafe.Offsetof(pipelineExecutableProperties{}.stages), 16},
		{"VkPipelineExecutablePropertiesKHR.name", unsafe.Offsetof(pipelineExecutableProperties{}.name), 20},
		{"VkPipelineExecutablePropertiesKHR.description", unsafe.Offsetof(pipelineExecutableProperties{}.description), 276},
		{"VkPipelineExecutablePropertiesKHR.subgroupSize", unsafe.Offsetof(pipelineExecutableProperties{}.subgroupSize), 532},
		{"VkPipelineExecutableInfoKHR.executableIndex", unsafe.Offsetof(pipelineExecutableInfo{}.executableIndex), 24},
		{"VkPipelineExecutableStatisticKHR.name", unsafe.Offsetof(pipelineExecutableStatistic{}.name), 16},
		{"VkPipelineExecutableStatisticKHR.description", unsafe.Offsetof(pipelineExecutableStatistic{}.description), 272},
		{"VkPipelineExecutableStatisticKHR.format", unsafe.Offsetof(pipelineExecutableStatistic{}.format), 528},
		{"VkPipelineExecutableStatisticKHR.value", unsafe.Offsetof(pipelineExecutableStatistic{}.value), 536},
	} {
		if s.got != s.want {
			t.Errorf("%s = %d, want %d", s.name, s.got, s.want)
		}
	}
}
