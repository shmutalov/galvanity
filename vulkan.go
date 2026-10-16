//go:build (windows || linux || darwin) && (amd64 || arm64)

package main

//go:generate glslc --target-env=vulkan1.0 -O shaders/cruncher.comp -o shaders/cruncher.spv

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	_ "embed"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"filippo.io/edwards25519"
	"filippo.io/edwards25519/field"
	"github.com/algorand/go-algorand-sdk/crypto"
	"golang.org/x/crypto/chacha20"

	"galvanity/internal/vk"
)

//go:embed shaders/cruncher.spv
var cruncherSPIRV []byte

const (
	gpuWorkgroupSize   = 64      // specialization constant 0 (local_size_x) of cruncher.comp
	gpuStartCandidates = 1 << 18 // first dispatch, before dispatchSize adapts it
	gpuMaxGroups       = 1 << 15 // workgroups per dispatch (Vulkan guarantees 65535)
	gpuMaxIterations   = 4096    // loop iterations per invocation per dispatch
	gpuMaxHits         = 4096    // matches recorded per dispatch
	gpuTargetBatch     = 200 * time.Millisecond
	gpuTimeout         = 30 * time.Second

	gpuModeSearch = 0
	gpuModeDump   = 1

	// Params buffer layout (std430, see cruncher.comp).
	paramsKey        = 0
	paramsNonce      = 32
	paramsSearchType = 40
	paramsPatternLen = 44
	paramsMode       = 48
	paramsMaxHits    = 52
	paramsPattern    = 56
	paramsSize       = paramsPattern + 4*addressLen

	resultsHits = 8 // hit_count, padding, then uvec2 hits[]
	pushSize    = 12
)

const vendorAMD = 0x1002

// gpuConfigFor picks the shader variant for a device. The batch and wave
// sizes were tuned on an AMD Radeon RX 7900 XT (RDNA3), where 8 candidates
// per invocation at wave32 is fastest even though some registers spill to
// scratch memory. Other GPUs get a batch of 2, which does not spill there and
// should stay within NVIDIA's 255 registers per thread.
func gpuConfigFor(info vk.DeviceInfo) gpuConfig {
	cfg := gpuConfig{window: 8, interleave: 1, batch: 2}
	if info.VendorID == vendorAMD {
		cfg.batch = 3 // the largest batch that does not spill at wave64
		if info.SubgroupSizeControl && info.MinSubgroupSize <= 32 && 32 <= info.MaxSubgroupSize {
			cfg.batch, cfg.subgroup = 8, 32
		}
	}
	return cfg
}

type vulkanBackend struct {
	device int        // index from -list-devices, or -1 to pick automatically
	config *gpuConfig // -gpu-config, or nil for gpuConfigFor's choice
}

func (b vulkanBackend) run(ctx context.Context, s search, found chan<- crypto.Account, checked *atomic.Uint64) error {
	inst, err := vk.NewInstance("galvanity")
	if err != nil {
		return err
	}
	defer inst.Destroy()

	index := b.device
	if index < 0 {
		if index, err = inst.BestDevice(); err != nil {
			return err
		}
	}
	devs := inst.Devices()
	if index >= len(devs) {
		return fmt.Errorf("no Vulkan device with index %d (found %d)", index, len(devs))
	}
	info := devs[index]
	cfg := gpuConfigFor(info)
	if b.config != nil {
		cfg = *b.config
	}
	// Compiling the shader can take several seconds on the first run.
	fmt.Printf("Vulkan device #%d: %s (%s), GPU settings %s, preparing shader...\n", info.Index, info.Name, info.Type, cfg)

	// A fresh secret key per run: seeds are unpredictable, and two hits
	// reveal nothing about each other.
	var key [32]byte
	var nonce [8]byte
	rand.Read(key[:])
	rand.Read(nonce[:])

	g, err := newGPUSearcher(inst, index, s, key, nonce, cfg, 0)
	if err != nil {
		return err
	}
	defer g.close()
	return g.search(ctx, s, found, checked, g.startSize())
}

// search checks candidates 0, 1, 2, ... in dispatches that start at size and
// adapt to gpuTargetBatch, until ctx is cancelled. Every hit is re-derived on
// the CPU and checked against s before it is sent to found; the hits of the
// last dispatch are sent even after a cancel (the receiver keeps reading
// until search returns).
func (g *gpuSearcher) search(ctx context.Context, s search, found chan<- crypto.Account, checked *atomic.Uint64, size dispatchSize) error {
	var base uint64
	warned := false
	for ctx.Err() == nil {
		start := time.Now()
		total, hits, err := g.run(base, size, gpuModeSearch)
		if err != nil {
			return err
		}
		elapsed := time.Since(start)

		for _, idx := range hits {
			acc, err := g.account(idx)
			if err != nil {
				return err
			}
			if addr := acc.Address.String(); !s.match(addr) {
				return fmt.Errorf("GPU reported candidate %d but its address %s does not match; the shader or driver miscomputed it", idx, addr)
			}
			found <- acc
		}
		if total > gpuMaxHits && !warned {
			fmt.Fprintf(os.Stderr, "warning: %d matches in one GPU batch, only %d per batch are reported; use a longer pattern\n", total, gpuMaxHits)
			warned = true
		}

		n := g.candidates(size)
		base += n
		checked.Add(n)
		size.adjust(elapsed)
	}
	return nil
}

func printVulkanDevices() error {
	inst, err := vk.NewInstance("galvanity")
	if err != nil {
		return err
	}
	defer inst.Destroy()
	devs := inst.Devices()
	if len(devs) == 0 {
		fmt.Println("No Vulkan devices found")
		return nil
	}
	best, _ := inst.BestDevice()
	for _, d := range devs {
		var notes []string
		if d.Index == best {
			notes = append(notes, "default")
		}
		if !d.ShaderInt64 {
			notes = append(notes, "unusable: no shaderInt64")
		}
		if !d.Compute {
			notes = append(notes, "unusable: no compute queue")
		}
		line := fmt.Sprintf("#%d: %s (%s), GPU settings %s", d.Index, d.Name, d.Type, gpuConfigFor(d))
		if len(notes) > 0 {
			line += " [" + strings.Join(notes, ", ") + "]"
		}
		fmt.Println(line)
	}
	return nil
}

// gpuSearcher owns the device resources for the cruncher shader.
type gpuSearcher struct {
	dev     *vk.Device
	kernel  *vk.Kernel
	table   *vk.Buffer
	params  *vk.Buffer
	results *vk.Buffer
	dump    *vk.Buffer
	cfg     gpuConfig
	key     [32]byte
	nonce   [8]byte
}

// newGPUSearcher prepares the shader variant cfg on device index.
// dumpCandidates sizes the buffer used by gpuModeDump (tests only); pass 0
// otherwise.
func newGPUSearcher(inst *vk.Instance, index int, s search, key [32]byte, nonce [8]byte, cfg gpuConfig, dumpCandidates int) (_ *gpuSearcher, err error) {
	dev, err := inst.Open(index, vk.DeviceOptions{SubgroupSizeControl: cfg.subgroup != 0, PipelineStatistics: cfg.stats})
	if err != nil {
		return nil, err
	}
	g := &gpuSearcher{dev: dev, cfg: cfg, key: key, nonce: nonce}
	defer func() {
		if err != nil {
			g.close()
		}
	}()

	table := baseTable(cfg.window, cfg.interleave)
	if g.table, err = dev.NewBuffer(4*len(table), true); err != nil {
		return nil, err
	}
	for i, v := range table {
		binary.LittleEndian.PutUint32(g.table.Data[4*i:], uint32(v))
	}

	if g.params, err = dev.NewBuffer(paramsSize, true); err != nil {
		return nil, err
	}
	p := g.params.Data
	copy(p[paramsKey:], key[:]) // the shader reads little-endian words, like ChaCha20
	copy(p[paramsNonce:], nonce[:])
	binary.LittleEndian.PutUint32(p[paramsMaxHits:], gpuMaxHits)
	g.setSearch(s)

	if g.results, err = dev.NewBuffer(resultsHits+8*gpuMaxHits, false); err != nil {
		return nil, err
	}
	if g.dump, err = dev.NewBuffer(max(36*dumpCandidates, 4), false); err != nil {
		return nil, err
	}
	if g.kernel, err = dev.NewKernel(cruncherSPIRV, []*vk.Buffer{g.table, g.params, g.results, g.dump}, pushSize,
		[]uint32{gpuWorkgroupSize, cfg.window, cfg.interleave, cfg.batch},
		vk.KernelOptions{SubgroupSize: cfg.subgroup, WorkgroupSize: gpuWorkgroupSize, CaptureStatistics: cfg.stats}); err != nil {
		return nil, err
	}
	return g, nil
}

// setSearch changes the pattern checked by later runs.
func (g *gpuSearcher) setSearch(s search) {
	p := g.params.Data
	binary.LittleEndian.PutUint32(p[paramsSearchType:], uint32(s.typ))
	binary.LittleEndian.PutUint32(p[paramsPatternLen:], uint32(len(s.pattern)))
	clear(p[paramsPattern:paramsSize])
	for i := range len(s.pattern) {
		binary.LittleEndian.PutUint32(p[paramsPattern+4*i:], uint32(strings.IndexByte(base32Alphabet, s.pattern[i])))
	}
}

func (g *gpuSearcher) close() {
	g.dev.WaitIdle() // a timed-out dispatch may still be using the buffers
	for _, b := range []*vk.Buffer{g.table, g.params, g.results, g.dump} {
		if b != nil {
			b.Destroy()
		}
	}
	if g.kernel != nil {
		g.kernel.Destroy()
	}
	g.dev.Destroy()
}

// dispatchSize is the work of one dispatch. adjust keeps dispatches near
// gpuTargetBatch: long enough to amortize submission overhead, short enough
// to stop promptly and to stay far from the OS GPU watchdog (2 s on
// Windows). It grows the workgroup count first, which shortens the partly
// idle last round of each dispatch, then the iterations per invocation.
type dispatchSize struct {
	groups, iterations uint32
}

func (d *dispatchSize) adjust(elapsed time.Duration) {
	switch {
	case elapsed < gpuTargetBatch/2:
		if d.groups < gpuMaxGroups {
			d.groups = min(2*d.groups, gpuMaxGroups)
		} else if d.iterations < gpuMaxIterations {
			d.iterations *= 2
		}
	case elapsed > gpuTargetBatch*2:
		if d.iterations > 1 {
			d.iterations /= 2
		} else if d.groups > 1 {
			d.groups /= 2
		}
	}
}

// startSize is the first dispatch: gpuStartCandidates whatever the batch.
func (g *gpuSearcher) startSize() dispatchSize {
	return dispatchSize{groups: max(gpuStartCandidates/(gpuWorkgroupSize*g.cfg.batch), 1), iterations: 1}
}

// candidates is the number of candidates a dispatch of size d checks.
func (g *gpuSearcher) candidates(d dispatchSize) uint64 {
	return uint64(d.groups) * gpuWorkgroupSize * uint64(d.iterations) * uint64(g.cfg.batch)
}

// run checks candidates base .. base+g.candidates(size)-1 and returns the
// total number of matches and the indices of the first gpuMaxHits of them.
// In gpuModeDump it writes the address of candidate base+o to dump words
// 9*o .. 9*o+8 instead.
func (g *gpuSearcher) run(base uint64, size dispatchSize, mode uint32) (uint32, []uint64, error) {
	binary.LittleEndian.PutUint32(g.params.Data[paramsMode:], mode)
	clear(g.results.Data[:resultsHits])

	push := make([]byte, pushSize)
	binary.LittleEndian.PutUint64(push[0:], base)
	binary.LittleEndian.PutUint32(push[8:], size.iterations)
	if err := g.dev.Dispatch(g.kernel, push, size.groups, gpuTimeout); err != nil {
		return 0, nil, err
	}

	r := g.results.Data
	total := binary.LittleEndian.Uint32(r)
	hits := make([]uint64, min(total, gpuMaxHits))
	for i := range hits {
		hits[i] = binary.LittleEndian.Uint64(r[resultsHits+8*i:])
	}
	return total, hits, nil
}

// account re-derives candidate idx on the CPU.
func (g *gpuSearcher) account(idx uint64) (crypto.Account, error) {
	seed := gpuSeed(&g.key, &g.nonce, idx)
	return crypto.AccountFromPrivateKey(ed25519.NewKeyFromSeed(seed[:]))
}

// gpuSeed returns the ed25519 seed of candidate idx, exactly as the shader
// derives it: the first 32 bytes of the ChaCha20 block with counter lo32(idx)
// and nonce hi32(idx) || nonce.
func gpuSeed(key *[32]byte, nonce *[8]byte, idx uint64) [32]byte {
	var n [chacha20.NonceSize]byte
	binary.LittleEndian.PutUint32(n[:], uint32(idx>>32))
	copy(n[4:], nonce[:])
	c, err := chacha20.NewUnauthenticatedCipher(key[:], n[:])
	if err != nil {
		panic(err)
	}
	c.SetCounter(uint32(idx))
	var seed [32]byte
	c.XORKeyStream(seed[:], seed[:])
	return seed
}

// baseTable returns the shader's precomputed points for window w and
// interleave t, 32 int32 (128 bytes, one cache line) per entry; see
// ge_scalarmult_base in cruncher.comp:
//
//	entries [0, 2^w)  start table: entry f = top + (f - 2^(w-1)) * 8B as
//	                  affine (x, y, x*y), top = c * 2^(3 + w (m - t + 1)) * B
//	then rows i < ceil(m/t) of 2^(w-1)+1 entries: entry j = j * 2^(t w i) * 8B
//	                  as (y+x, y-x, 2dxy), entry 0 being the identity
//
// m = ceil(252/w) is the number of signed digits of the scalar >> 3, and c
// the constant carry above them (1 iff w divides 252).
func baseTable(w, t uint32) []int32 {
	m := (252 + w - 1) / w
	// The recoding needs a constant top carry (252 mod w != w-1) and all
	// digits within the shader's 256-bit register (w*m <= 256).
	if w < 4 || w > 8 || (t != 1 && t != 2) || 252%w == w-1 || w*m > 256 {
		panic(fmt.Sprintf("baseTable: unsupported window %d/%d", w, t))
	}
	d := new(field.Element).Multiply(
		new(field.Element).Negate(feSmall(121665)),
		new(field.Element).Invert(feSmall(121666)),
	)
	d2 := new(field.Element).Add(d, d)

	half := uint32(1) << (w - 1)
	rows := (m + t - 1) / t
	table := make([]int32, 0, 32*(1<<w+rows*(half+1)))
	emit := func(fes ...*field.Element) {
		for _, fe := range fes {
			limbs := feLimbs(fe)
			table = append(table, limbs[:]...)
		}
		table = append(table, 0, 0) // pad to 128 bytes
	}
	affine := func(p *edwards25519.Point) (x, y *field.Element) {
		X, Y, Z, _ := p.ExtendedCoordinates()
		zInv := new(field.Element).Invert(Z)
		return new(field.Element).Multiply(X, zInv), new(field.Element).Multiply(Y, zInv)
	}
	double := func(p *edwards25519.Point, n uint32) *edwards25519.Point {
		r := new(edwards25519.Point).Set(p)
		for range n {
			r.Add(r, r)
		}
		return r
	}

	b8 := double(edwards25519.NewGeneratorPoint(), 3)
	p := edwards25519.NewIdentityPoint()
	if 252%w == 0 {
		p = double(edwards25519.NewGeneratorPoint(), 3+w*(m-t+1))
	}
	neg := new(edwards25519.Point).Negate(b8)
	for range half {
		p.Add(p, neg) // top - half*8B
	}
	for range 1 << w {
		x, y := affine(p)
		emit(x, y, new(field.Element).Multiply(x, y))
		p.Add(p, b8)
	}

	row := b8
	for range rows {
		p := edwards25519.NewIdentityPoint()
		for j := range half + 1 {
			if j > 0 {
				p.Add(p, row)
			}
			x, y := affine(p)
			xy2d := new(field.Element).Multiply(x, y)
			xy2d.Multiply(xy2d, d2)
			emit(new(field.Element).Add(y, x), new(field.Element).Subtract(y, x), xy2d)
		}
		row = double(row, t*w)
	}
	return table
}

func feSmall(v uint32) *field.Element {
	var b [32]byte
	binary.LittleEndian.PutUint32(b[:], v)
	fe, err := new(field.Element).SetBytes(b[:])
	if err != nil {
		panic(err)
	}
	return fe
}

// feLimbs splits a field element into the shader's representation: limb i
// holds bits ceil(25.5*i) .. ceil(25.5*(i+1))-1 (alternately 26 and 25 bits).
func feLimbs(fe *field.Element) [10]int32 {
	b := fe.Bytes()
	var words [5]uint64
	for i := range 4 {
		words[i] = binary.LittleEndian.Uint64(b[8*i:])
	}
	var limbs [10]int32
	bit := 0
	for i := range limbs {
		width := 26 - i%2
		w, sh := bit/64, bit%64
		v := words[w] >> sh
		if sh+width > 64 {
			v |= words[w+1] << (64 - sh)
		}
		limbs[i] = int32(v & (1<<width - 1))
		bit += width
	}
	return limbs
}
