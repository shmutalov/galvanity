//go:build (windows || linux || darwin) && (amd64 || arm64)

package main

import (
	"context"
	"encoding/base32"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/algorand/go-algorand-sdk/crypto"

	"galvanity/internal/vk"
)

var (
	testKey   = [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	testNonce = [8]byte{0xde, 0xad, 0xbe, 0xef, 0x01, 0x23, 0x45, 0x67}
)

// testDevice returns the device the vulkan backend would pick by default.
// Without one the test is skipped, unless GALVANITY_REQUIRE_VULKAN=1 (set in
// CI on Linux, where lavapipe is installed for these tests).
func testDevice(t testing.TB) (*vk.Instance, int) {
	t.Helper()
	skip := t.Skipf
	if os.Getenv("GALVANITY_REQUIRE_VULKAN") == "1" {
		skip = t.Fatalf
	}
	inst, err := vk.NewInstance("galvanity-test")
	if err != nil {
		skip("Vulkan unavailable: %v", err)
	}
	t.Cleanup(inst.Destroy)
	index, err := inst.BestDevice()
	if err != nil {
		skip("no usable Vulkan device: %v", err)
	}
	t.Logf("using %s (%s)", inst.Devices()[index].Name, inst.Devices()[index].Type)
	return inst, index
}

// cpuAddresses derives the addresses of candidates base..base+n-1 on the CPU.
func cpuAddresses(t *testing.T, g *gpuSearcher, base uint64, n int) []string {
	t.Helper()
	addrs := make([]string, n)
	for i := range addrs {
		acc, err := g.account(base + uint64(i))
		if err != nil {
			t.Fatal(err)
		}
		addrs[i] = acc.Address.String()
	}
	return addrs
}

// testGPUConfigs are the variants gpuConfigFor ships, for this device and
// the others, plus one with the doubling pass (T=2), an odd window and a
// batch of 3 (which catches index mistakes powers of two hide), and, where the
// device allows it, one with a required subgroup size. TestBaseTable covers
// every window and interleave on the CPU.
func testGPUConfigs(info vk.DeviceInfo) []gpuConfig {
	own := gpuConfigFor(info)
	cfgs := []gpuConfig{
		own,
		{window: 8, interleave: 1, batch: 2}, // GPUs other than AMD
		{window: 8, interleave: 1, batch: 3}, // AMD without subgroup size control
		{window: 8, interleave: 1, batch: 8}, // AMD (with wave32 when available)
		{window: 5, interleave: 2, batch: 3},
	}
	if info.SubgroupSizeControl && own.subgroup == 0 {
		cfgs = append(cfgs, gpuConfig{window: 8, interleave: 1, batch: 2, subgroup: info.MinSubgroupSize})
	}
	var unique []gpuConfig
	for _, c := range cfgs {
		if !slices.Contains(unique, c) {
			unique = append(unique, c)
		}
	}
	return unique
}

func TestGPUDerivesAddresses(t *testing.T) {
	inst, index := testDevice(t)

	// The same candidates for every batch size: groups*64*2*batch = n.
	const n = gpuWorkgroupSize * 2 * 48
	// Straddle a 2^32 boundary to exercise the 64-bit candidate index.
	const base = 1<<32 - n/2

	var want []string
	for _, cfg := range testGPUConfigs(inst.Devices()[index]) {
		t.Run(configName(cfg), func(t *testing.T) {
			if 48%cfg.batch != 0 {
				t.Fatalf("batch %d does not divide 48", cfg.batch)
			}
			g, err := newGPUSearcher(inst, index, search{typ: searchStarts, pattern: "A"}, testKey, testNonce, cfg, n)
			if err != nil {
				t.Fatal(err)
			}
			defer g.close()
			size := dispatchSize{groups: 48 / cfg.batch, iterations: 2}
			if g.candidates(size) != n {
				t.Fatalf("dispatch covers %d candidates, want %d", g.candidates(size), n)
			}
			if _, _, err := g.run(base, size, gpuModeDump); err != nil {
				t.Fatal(err)
			}

			if want == nil {
				want = cpuAddresses(t, g, base, n)
			}
			enc := base32.StdEncoding.WithPadding(base32.NoPadding)
			bad := 0
			for i, w := range want {
				got := enc.EncodeToString(g.dump.Data[36*i : 36*i+36])
				if got != w {
					if bad < 5 {
						t.Errorf("candidate %d: GPU address %s, want %s", base+uint64(i), got, w)
					}
					bad++
				}
			}
			if bad > 0 {
				t.Errorf("%d of %d addresses differ", bad, n)
			}
		})
	}
}

func TestGPUSearch(t *testing.T) {
	inst, index := testDevice(t)

	// Hits on both sides of 2^32 exercise both words of the reported indices.
	const base = 1<<32 - 8000

	cfg := gpuConfigFor(inst.Devices()[index])
	g, err := newGPUSearcher(inst, index, search{typ: searchStarts, pattern: "A"}, testKey, testNonce, cfg, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer g.close()
	size := dispatchSize{groups: 256 / cfg.batch, iterations: 1}
	n := int(g.candidates(size))
	addrs := cpuAddresses(t, g, base, n) // the key is fixed, so derive these once

	// Address characters 0..50 depend only on the public key, 51 mixes in the
	// checksum, and 52..57 are checksum only; the long patterns cover each.
	a, b := addrs[100], addrs[n-7]
	for _, s := range []search{
		{searchStarts, "A"},
		{searchStarts, "AB"},
		{searchEnds, "Q"},
		{searchEnds, "7A"},
		{searchContains, "AB"},
		{searchContains, "Z2"},
		{searchExact, addrs[n/3]},
		{searchContains, a[10:19]},
		{searchContains, a[45:54]},
		{searchContains, b[50:58]},
		{searchContains, b[52:58]},
		{searchEnds, a[48:]},
		{searchEnds, b[51:]},
		{searchStarts, a[:53]},
		{searchStarts, b[:51]},
		{searchStarts, a[:52]}, // the shortest prefix that needs the checksum
		{searchStarts, b[:52]},
		{searchContains, "A"}, // more matches than gpuMaxHits
	} {
		t.Run(s.typ.String()+"_"+s.pattern, func(t *testing.T) {
			var want []uint64
			for i, a := range addrs {
				if s.match(a) {
					want = append(want, base+uint64(i))
				}
			}
			g.setSearch(s)
			total, got, err := g.run(base, size, gpuModeSearch)
			if err != nil {
				t.Fatal(err)
			}
			slices.Sort(got)
			if len(want) > gpuMaxHits {
				// Only the first gpuMaxHits are reported, in no particular order.
				if int(total) != len(want) || len(got) != gpuMaxHits {
					t.Fatalf("GPU found %d matches and reported %d, want %d and %d", total, len(got), len(want), gpuMaxHits)
				}
				for i, x := range got {
					if _, ok := slices.BinarySearch(want, x); !ok || i > 0 && got[i-1] == x {
						t.Fatalf("reported hit %d is a duplicate or not a match", x)
					}
				}
				return
			}
			if int(total) != len(want) || !slices.Equal(got, want) {
				t.Errorf("GPU found %d matches %v, want %d %v", total, head(got), len(want), head(want))
			}
		})
	}
}

// TestGPUHitOverflow checks that the shader writes no more than max_hits
// hits: it lowers max_hits and checks that the rest of the results buffer
// keeps its sentinel bytes.
func TestGPUHitOverflow(t *testing.T) {
	inst, index := testDevice(t)
	cfg := gpuConfigFor(inst.Devices()[index])
	s := search{searchContains, "A"}
	g, err := newGPUSearcher(inst, index, s, testKey, testNonce, cfg, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer g.close()

	const base, maxHits = 1000, 16
	binary.LittleEndian.PutUint32(g.params.Data[paramsMaxHits:], maxHits)
	tail := g.results.Data[resultsHits+8*maxHits:]
	for i := range tail {
		tail[i] = 0xa5
	}
	size := dispatchSize{groups: 64 / cfg.batch, iterations: 1}
	total, _, err := g.run(base, size, gpuModeSearch)
	if err != nil {
		t.Fatal(err)
	}

	addrs := cpuAddresses(t, g, base, int(g.candidates(size)))
	want := 0
	for _, a := range addrs {
		if s.match(a) {
			want++
		}
	}
	if int(total) != want || total <= maxHits {
		t.Fatalf("GPU counted %d matches, want %d (more than %d)", total, want, maxHits)
	}
	seen := map[uint64]bool{}
	for i := range maxHits {
		idx := binary.LittleEndian.Uint64(g.results.Data[resultsHits+8*i:])
		if idx < base || idx-base >= uint64(len(addrs)) || !s.match(addrs[idx-base]) || seen[idx] {
			t.Fatalf("hit slot %d holds %d, which is not a distinct match", i, idx)
		}
		seen[idx] = true
	}
	for i, b := range tail {
		if b != 0xa5 {
			t.Fatalf("results byte %d was written although max_hits is %d", resultsHits+8*maxHits+i, maxHits)
		}
	}
}

// TestGPUSearchLoop runs the backend's dispatch loop until it has checked a
// few thousand candidates and checks that it reported every match among the
// candidates it counted, exactly once.
func TestGPUSearchLoop(t *testing.T) {
	inst, index := testDevice(t)
	cfg := gpuConfigFor(inst.Devices()[index])
	s := search{searchStarts, "A"}
	g, err := newGPUSearcher(inst, index, s, testKey, testNonce, cfg, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer g.close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	found := make(chan crypto.Account, 64)
	done := make(chan error, 1)
	var checked atomic.Uint64
	// Start small; the loop grows fast dispatches, so this takes a few of them.
	go func() { done <- g.search(ctx, s, found, &checked, dispatchSize{groups: 4, iterations: 1}) }()

	const enough = 1 << 14
	var reported []string
	for running := true; running; {
		select {
		case acc := <-found:
			reported = append(reported, acc.Address.String())
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
			running = false
		case <-time.After(time.Millisecond):
		}
		if checked.Load() >= enough {
			cancel()
		}
	}
	for len(found) > 0 {
		reported = append(reported, (<-found).Address.String())
	}

	var want []string
	for _, a := range cpuAddresses(t, g, 0, int(checked.Load())) {
		if s.match(a) {
			want = append(want, a)
		}
	}
	slices.Sort(want)
	slices.Sort(reported)
	if !slices.Equal(reported, want) {
		t.Errorf("the loop reported %d matches, want the %d among the %d candidates it counted", len(reported), len(want), checked.Load())
	}
}

// TestGPUPipelineStatistics checks the optional statistics path where the
// driver supports it (lavapipe does not).
func TestGPUPipelineStatistics(t *testing.T) {
	inst, index := testDevice(t)
	info := inst.Devices()[index]
	if !info.PipelineStatistics {
		t.Skip("the device does not report pipeline statistics")
	}
	cfg := gpuConfigFor(info)
	cfg.stats = true
	g, err := newGPUSearcher(inst, index, search{searchStarts, "A"}, testKey, testNonce, cfg, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer g.close()
	exes, err := g.kernel.Executables()
	if err != nil {
		t.Fatal(err)
	}
	if len(exes) == 0 || len(exes[0].Statistics) == 0 {
		t.Fatal("the driver reported no statistics")
	}
	for _, st := range exes[0].Statistics {
		t.Logf("%s: %s = %v", exes[0].Name, st.Name, st.Value)
	}
}

func head(s []uint64) []uint64 { return s[:min(len(s), 8)] }

var (
	benchConfigs = flag.String("gpu.configs", "", "shader variants for BenchmarkGPU as W/T/K[/subgroup], e.g. 8/1/1,8/1/2/32 (default: the device's own)")
	benchStats   = flag.Bool("gpu.stats", false, "report pipeline statistics (registers, scratch) in BenchmarkGPU")
)

// BenchmarkGPU measures search throughput on the default device, excluding
// pipeline compilation and host-side hit verification:
//
//	go test -run '^$' -bench GPU -benchtime 20x [-gpu.configs 8/1/1,8/1/2]
func BenchmarkGPU(b *testing.B) {
	inst, index := testDevice(b)
	cfgs := []gpuConfig{gpuConfigFor(inst.Devices()[index])}
	if *benchConfigs != "" {
		cfgs = nil
		for _, f := range strings.Split(*benchConfigs, ",") {
			cfg, err := parseGPUConfig(f)
			if err != nil {
				b.Fatal(err)
			}
			cfgs = append(cfgs, cfg)
		}
	}
	for _, cfg := range cfgs {
		cfg.stats = *benchStats
		b.Run(configName(cfg), func(b *testing.B) { benchGPU(b, inst, index, cfg) })
	}
}

// configName names a variant in subtest names (the / of String would nest them).
func configName(cfg gpuConfig) string {
	name := fmt.Sprintf("W=%d,T=%d,K=%d", cfg.window, cfg.interleave, cfg.batch)
	if cfg.subgroup != 0 {
		name += fmt.Sprintf(",wave=%d", cfg.subgroup)
	}
	return name
}

func benchGPU(b *testing.B, inst *vk.Instance, index int, cfg gpuConfig) {
	info := inst.Devices()[index]
	if cfg.subgroup != 0 && (!info.SubgroupSizeControl || cfg.subgroup < info.MinSubgroupSize || cfg.subgroup > info.MaxSubgroupSize) {
		b.Skipf("subgroup size %d is not supported", cfg.subgroup)
	}
	if cfg.stats && !info.PipelineStatistics {
		b.Log("pipeline statistics are not supported")
		cfg.stats = false
	}
	g, err := newGPUSearcher(inst, index, search{typ: searchStarts, pattern: "ALGOXYZ"}, testKey, testNonce, cfg, 0)
	if err != nil {
		b.Fatal(err)
	}
	defer g.close()
	if cfg.stats {
		exes, err := g.kernel.Executables()
		if err != nil {
			b.Fatal(err)
		}
		for _, e := range exes {
			for _, s := range e.Statistics {
				b.Logf("%s: %s = %v", e.Name, s.Name, s.Value)
			}
		}
	}

	// Let the dispatch size settle the way the backend's does.
	var base uint64
	size := g.startSize()
	for range 30 {
		start := time.Now()
		if _, _, err := g.run(base, size, gpuModeSearch); err != nil {
			b.Fatal(err)
		}
		base += g.candidates(size)
		size.adjust(time.Since(start))
	}

	var total uint64
	for b.Loop() {
		if _, _, err := g.run(base, size, gpuModeSearch); err != nil {
			b.Fatal(err)
		}
		base += g.candidates(size)
		total += g.candidates(size)
	}
	b.ReportMetric(float64(total)/b.Elapsed().Seconds()/1e6, "Mcand/s")
	b.ReportMetric(float64(size.groups), "groups")
	b.ReportMetric(float64(size.iterations), "iters")
}
