package main

import "testing"

func TestParseGPUConfig(t *testing.T) {
	valid := []struct {
		in   string
		want gpuConfig
	}{
		{"8/1/2", gpuConfig{window: 8, interleave: 1, batch: 2}},
		{"8/1/8/32", gpuConfig{window: 8, interleave: 1, batch: 8, subgroup: 32}},
		{"4/2/1", gpuConfig{window: 4, interleave: 2, batch: 1}},
		{"5/2/16/64", gpuConfig{window: 5, interleave: 2, batch: 16, subgroup: 64}},
	}
	for _, c := range valid {
		got, err := parseGPUConfig(c.in)
		if err != nil || got != c.want {
			t.Errorf("parseGPUConfig(%q) = %+v, %v; want %+v", c.in, got, err, c.want)
		}
		if got.String() != c.in {
			t.Errorf("%+v.String() = %q, want %q", got, got.String(), c.in)
		}
	}

	for _, in := range []string{
		"", "8", "8/1", "8/1/2/32/1", "8/1/x", "8/-1/2", " 8/1/2",
		"3/1/2", "9/1/2", // window
		"8/0/2", "8/3/2", // interleave
		"8/1/0", "8/1/17", // batch
		"8/1/2/0", "8/1/2/48", // subgroup
		"8/1/99999999999", "8/1/2/4294967296", // out of range
	} {
		if got, err := parseGPUConfig(in); err == nil {
			t.Errorf("parseGPUConfig(%q) = %+v, want an error", in, got)
		}
	}
}
