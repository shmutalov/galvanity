//go:build (windows || linux || darwin) && (amd64 || arm64)

package main

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math/big"
	"testing"

	"filippo.io/edwards25519"
	"filippo.io/edwards25519/field"
)

// validGPUConfigs lists every window/interleave pair baseTable supports.
func validGPUConfigs() []gpuConfig {
	var cfgs []gpuConfig
	for w := uint32(4); w <= 8; w++ {
		for t := uint32(1); t <= 2; t++ {
			cfgs = append(cfgs, gpuConfig{window: w, interleave: t})
		}
	}
	return cfgs
}

// TestBaseTable checks baseTable and the shader's digit logic without a GPU:
// it decodes the table words back to points and replays ge_scalarmult_base
// (shift register, bias words, start table, rows, doublings) with
// edwards25519 point arithmetic, comparing against ScalarBaseMult.
func TestBaseTable(t *testing.T) {
	// Random clamped scalars plus byte patterns that hit extreme digits.
	var scalars [][32]byte
	for _, b := range []byte{0x00, 0xff, 0x55, 0xaa, 0x0f, 0xf0, 0x80, 0x7f, 0x01, 0xfe} {
		var s [32]byte
		for i := range s {
			s[i] = b
		}
		scalars = append(scalars, s)
	}
	for range 50 {
		var s [32]byte
		rand.Read(s[:])
		scalars = append(scalars, s)
	}

	for _, cfg := range validGPUConfigs() {
		t.Run(fmt.Sprintf("W=%d,T=%d", cfg.window, cfg.interleave), func(t *testing.T) {
			table := baseTable(cfg.window, cfg.interleave)
			m := (252 + cfg.window - 1) / cfg.window
			half := uint32(1) << (cfg.window - 1)
			rows := (m + cfg.interleave - 1) / cfg.interleave
			if want := 32 * (1<<cfg.window + int(rows*(half+1))); len(table) != want {
				t.Fatalf("table has %d words, want %d", len(table), want)
			}
			for _, s := range scalars {
				s[0] &= 248
				s[31] = s[31]&127 | 64
				var a [8]uint32
				for i := range a {
					a[i] = binary.LittleEndian.Uint32(s[4*i:])
				}
				got := emulateScalarMultBase(t, table, cfg, a)
				sc, err := edwards25519.NewScalar().SetBytesWithClamping(s[:])
				if err != nil {
					t.Fatal(err)
				}
				if want := new(edwards25519.Point).ScalarBaseMult(sc); got.Equal(want) != 1 {
					t.Fatalf("scalar %x: emulated result differs from ScalarBaseMult", s)
				}
			}
		})
	}
}

// TestBiasWords checks the shader's C_WORD formula against the bias C built
// bit by bit (bits W-1, 2W-1, ... below 256).
func TestBiasWords(t *testing.T) {
	for w := uint32(4); w <= 8; w++ {
		c := new(big.Int)
		for k := uint32(0); w*k+w-1 < 256; k++ {
			c.SetBit(c, int(w*k+w-1), 1)
		}
		for j := uint32(0); j < 8; j++ {
			want := uint32(new(big.Int).Rsh(c, uint(32*j)).Uint64())
			if got := biasWord(w, j); got != want {
				t.Errorf("W=%d word %d: %08x, want %08x", w, j, got, want)
			}
		}
	}
}

// biasWord mirrors C_WORD in cruncher.comp.
func biasWord(w, j uint32) uint32 {
	mask := uint32(1)<<w - 1
	q := uint32(0xffffffff) / mask
	shift := (w - 1 - (32*j)%w + w - 32%w) % w
	return q<<shift | q>>(w-shift)
}

// emulateScalarMultBase replays the shader's ge_scalarmult_base on table.
func emulateScalarMultBase(t *testing.T, table []int32, cfg gpuConfig, a [8]uint32) *edwards25519.Point {
	t.Helper()
	w, tt := cfg.window, cfg.interleave
	half, mask := uint32(1)<<(w-1), uint32(1)<<w-1
	m := (252 + w - 1) / w
	startEntries, rowEntries := uint32(1)<<w, half+1

	reg := a
	shr256(&reg, 3)
	var carry uint64
	for j := range reg {
		s := uint64(reg[j]) + uint64(biasWord(w, uint32(j))) + carry
		reg[j], carry = uint32(s), s>>32
	}
	y := reg

	addDigit := func(h *edwards25519.Point, row, f uint32) {
		e := int32(f) - int32(half)
		p := tableEntry(t, table, startEntries+row*rowEntries+uint32(max(e, -e)), false)
		if e < 0 {
			p.Negate(p)
		}
		h.Add(h, p)
	}

	if tt == 2 {
		shr256(&reg, w)
	}
	h := tableEntry(t, table, reg[0]&mask, true)
	shr256(&reg, tt*w)
	for i := uint32(1); i < m/tt; i++ {
		addDigit(h, i, reg[0]&mask)
		shr256(&reg, tt*w)
	}
	if tt == 2 {
		for range w {
			h.Add(h, h)
		}
		reg = y
		for i := uint32(0); i < (m+1)/2; i++ {
			addDigit(h, i, reg[0]&mask)
			shr256(&reg, tt*w)
		}
	}
	return h
}

func shr256(v *[8]uint32, s uint32) {
	for i := range 7 {
		v[i] = v[i]>>s | v[i+1]<<(32-s)
	}
	v[7] >>= s
}

// tableEntry decodes entry e: affine (x, y, xy) for start entries, otherwise
// (y+x, y-x, 2dxy). It checks the redundant third element too.
func tableEntry(t *testing.T, table []int32, e uint32, start bool) *edwards25519.Point {
	t.Helper()
	o := 32 * e
	f0, f1, f2 := limbsToField(table[o:o+10]), limbsToField(table[o+10:o+20]), limbsToField(table[o+20:o+30])
	x, y := f0, f1
	if !start {
		inv2 := new(field.Element).Invert(feSmall(2))
		y = new(field.Element).Multiply(new(field.Element).Add(f0, f1), inv2)
		x = new(field.Element).Multiply(new(field.Element).Subtract(f0, f1), inv2)
	}
	xy := new(field.Element).Multiply(x, y)
	if !start {
		d := new(field.Element).Multiply(new(field.Element).Negate(feSmall(121665)), new(field.Element).Invert(feSmall(121666)))
		xy.Multiply(xy, new(field.Element).Add(d, d))
	}
	if xy.Equal(f2) != 1 {
		t.Fatalf("table entry %d: inconsistent third element", e)
	}
	enc := y.Bytes()
	enc[31] |= byte(x.IsNegative()) << 7
	p, err := new(edwards25519.Point).SetBytes(enc)
	if err != nil {
		t.Fatalf("table entry %d is not on the curve: %v", e, err)
	}
	return p
}

// limbsToField inverts feLimbs (limb i at bit ceil(25.5*i)).
func limbsToField(limbs []int32) *field.Element {
	v := new(big.Int)
	bit := 0
	for i, l := range limbs {
		v.Add(v, new(big.Int).Lsh(big.NewInt(int64(l)), uint(bit)))
		bit += 26 - i%2
	}
	p := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 255), big.NewInt(19))
	v.Mod(v, p)
	var b [32]byte
	v.FillBytes(b[:])
	for i := range 16 {
		b[i], b[31-i] = b[31-i], b[i]
	}
	fe, err := new(field.Element).SetBytes(b[:])
	if err != nil {
		panic(err)
	}
	return fe
}
