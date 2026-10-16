package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// gpuConfig selects a shader variant through specialization constants
// 1.. of cruncher.comp; baseTable builds the matching table.
type gpuConfig struct {
	window     uint32 // W: bits per signed scalar digit, 4..8
	interleave uint32 // T: 1 = a table row per digit, 2 = half the rows plus W doublings
	batch      uint32 // K: candidates per invocation sharing one field inversion

	subgroup uint32 // required subgroup (wave) size, 0 = driver default
	stats    bool   // capture pipeline statistics (benchmarks)
}

const gpuMaxBatch = 16

// parseGPUConfig parses the -gpu-config syntax W/T/K or W/T/K/S, for
// example 8/1/8/32: window bits, interleave, candidates per invocation and
// an optional required subgroup (wave) size. Whether the device supports
// the subgroup size is checked when the shader is built.
func parseGPUConfig(s string) (gpuConfig, error) {
	fields := strings.Split(s, "/")
	if len(fields) != 3 && len(fields) != 4 {
		return gpuConfig{}, fmt.Errorf("invalid GPU settings %q, want window/interleave/batch[/subgroup], e.g. 8/1/2", s)
	}
	ranges := [4]string{
		"window must be 4..8",
		"interleave must be 1 or 2",
		fmt.Sprintf("batch must be 1..%d", gpuMaxBatch),
		"subgroup size must be a power of two",
	}
	invalid := func(field int) (gpuConfig, error) {
		return gpuConfig{}, fmt.Errorf("invalid GPU settings %q: %s", s, ranges[field])
	}
	var v [4]uint32
	for i, f := range fields {
		n, err := strconv.ParseUint(f, 10, 32)
		if errors.Is(err, strconv.ErrRange) {
			return invalid(i)
		}
		if err != nil {
			return gpuConfig{}, fmt.Errorf("invalid GPU settings %q: %q is not a number", s, f)
		}
		v[i] = uint32(n)
	}
	cfg := gpuConfig{window: v[0], interleave: v[1], batch: v[2], subgroup: v[3]}
	switch {
	case cfg.window < 4 || cfg.window > 8:
		return invalid(0)
	case cfg.interleave != 1 && cfg.interleave != 2:
		return invalid(1)
	case cfg.batch < 1 || cfg.batch > gpuMaxBatch:
		return invalid(2)
	case len(fields) == 4 && (cfg.subgroup == 0 || cfg.subgroup&(cfg.subgroup-1) != 0):
		return invalid(3)
	}
	return cfg, nil
}

// String formats the settings in parseGPUConfig's syntax.
func (c gpuConfig) String() string {
	s := fmt.Sprintf("%d/%d/%d", c.window, c.interleave, c.batch)
	if c.subgroup != 0 {
		s += fmt.Sprintf("/%d", c.subgroup)
	}
	return s
}
