//go:build (windows || linux || darwin) && (amd64 || arm64)

package vk

import (
	"errors"
	"fmt"
	"math"
	"runtime"

	"github.com/ebitengine/purego"
)

// Optional commands: nil when the loader does not provide them. Callers
// gate every use on the matching DeviceInfo / Device flag.
var (
	vkEnumerateInstanceVersion           func(version *uint32) Result
	vkGetPhysicalDeviceFeatures2         func(pd uintptr, features *physicalDeviceFeatures2)
	vkGetPhysicalDeviceProperties2       func(pd uintptr, props *physicalDeviceProperties2)
	vkGetPipelineExecutablePropertiesKHR func(device uintptr, info *pipelineInfo, count *uint32, props *pipelineExecutableProperties) Result
	vkGetPipelineExecutableStatisticsKHR func(device uintptr, info *pipelineExecutableInfo, count *uint32, stats *pipelineExecutableStatistic) Result
)

func resolveOptional(instance uintptr, fns map[string]any) {
	for name, fptr := range fns {
		if addr := vkGetInstanceProcAddr(instance, name+"\x00"); addr != 0 {
			purego.RegisterFunc(fptr, addr)
		}
	}
}

// Statistic is one VkPipelineExecutableStatisticKHR. Value is a bool, int64,
// uint64 or float64, depending on the driver-reported format.
type Statistic struct {
	Name, Description string
	Value             any
}

// Executable describes one compiled executable of a pipeline.
type Executable struct {
	Name, Description string
	Stages            uint32
	SubgroupSize      uint32
	Statistics        []Statistic
}

// Executables returns the statistics of the kernel's pipeline, such as
// register counts and scratch memory. The device must have been opened with
// DeviceOptions.PipelineStatistics and the kernel built with
// KernelOptions.CaptureStatistics.
func (k *Kernel) Executables() ([]Executable, error) {
	d := k.dev
	if !d.Options.PipelineStatistics || !k.captured {
		return nil, errors.New("pipeline statistics were not captured for this kernel")
	}
	var pin runtime.Pinner
	defer pin.Unpin()

	pinfo := &pipelineInfo{sType: stPipelineInfo, pipeline: k.pipeline}
	var n uint32
	if err := check(vkGetPipelineExecutablePropertiesKHR(d.handle, pinfo, &n, nil), "vkGetPipelineExecutablePropertiesKHR"); err != nil || n == 0 {
		return nil, err
	}
	props := make([]pipelineExecutableProperties, n)
	for i := range props {
		props[i].sType = stPipelineExecutableProperties
	}
	if err := check(vkGetPipelineExecutablePropertiesKHR(d.handle, pinfo, &n, &props[0]), "vkGetPipelineExecutablePropertiesKHR"); err != nil {
		return nil, err
	}

	out := make([]Executable, 0, n)
	for i, p := range props[:n] {
		e := Executable{
			Name:         cstring(p.name[:]),
			Description:  cstring(p.description[:]),
			Stages:       p.stages,
			SubgroupSize: p.subgroupSize,
		}
		einfo := &pipelineExecutableInfo{sType: stPipelineExecutableInfo, pipeline: k.pipeline, executableIndex: uint32(i)}

		var sn uint32
		if err := check(vkGetPipelineExecutableStatisticsKHR(d.handle, einfo, &sn, nil), "vkGetPipelineExecutableStatisticsKHR"); err != nil {
			return nil, err
		}
		if sn > 0 {
			stats := make([]pipelineExecutableStatistic, sn)
			for j := range stats {
				stats[j].sType = stPipelineExecutableStatistic
			}
			if err := check(vkGetPipelineExecutableStatisticsKHR(d.handle, einfo, &sn, &stats[0]), "vkGetPipelineExecutableStatisticsKHR"); err != nil {
				return nil, err
			}
			for _, s := range stats[:sn] {
				e.Statistics = append(e.Statistics, Statistic{
					Name:        cstring(s.name[:]),
					Description: cstring(s.description[:]),
					Value:       statValue(s.format, s.value),
				})
			}
		}
		out = append(out, e)
	}
	return out, nil
}

func statValue(format uint32, v uint64) any {
	switch format {
	case statFormatBool32:
		return uint32(v) != 0 // b32 is the first member of the union (little-endian)
	case statFormatInt64:
		return int64(v)
	case statFormatUint64:
		return v
	case statFormatFloat64:
		return math.Float64frombits(v)
	}
	return fmt.Sprintf("format %d: %#x", format, v)
}
