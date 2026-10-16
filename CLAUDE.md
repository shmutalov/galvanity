# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Galvanity is a CLI vanity-address generator for Algorand, written in Go (module `galvanity`, `go 1.27.2`). It brute-forces Algorand accounts until the address matches a BASE32 pattern, either on CPU cores or on a GPU through Vulkan.

## Commands

```
go build                       # produces ./galvanity (galvanity.exe on Windows)
go run . -backend vulkan starts ALGO
go run . -list-devices
go vet ./...
go test ./...                  # GPU tests skip without a Vulkan device (GALVANITY_REQUIRE_VULKAN=1 fails instead)
go test -run TestGPUSearch -v .
go test ./internal/vk          # Vulkan struct layout tests
go generate                    # rebuild shaders/cruncher.spv (needs glslc from the Vulkan SDK)
go test -run '^$' -bench GPU -benchtime 20x [-gpu.configs 8/1/2,8/1/8/32] [-gpu.stats]
```

`go build ./...` builds two packages and therefore writes no binary; CI uses `go build -v .` before uploading `./galvanity*` as artifacts. CI (`.github/workflows/build.yml`) runs on ubuntu/macos/windows; on Linux it installs Mesa's lavapipe and sets `GALVANITY_REQUIRE_VULKAN=1`, so the GPU tests must run there against a software Vulkan driver.

`BenchmarkGPU` measures GPU throughput without shader compilation or CPU verification. Use it, not the CLI's progress line, to compare shader changes; `-gpu.configs` takes `window/interleave/batch[/subgroup]` entries and `-gpu.stats` logs register and scratch use where the driver reports them (AMD does, lavapipe does not). The GPU is shared, so never run benchmarks concurrently.

## Architecture

- `main.go`: flags (`-backend`, `-device`, `-list-devices`, `-out`, `-gpu-config`, which must precede the positional args), the progress/printing loop, and the `backend` interface. Backends send matches on a channel and add to an atomic counter; `main` does all output and keeps reading `found` until the backend returns, so backends may send unconditionally.
- `output.go`: `accountRecord` is the found-key block (stdout by default). With `-out`, a `keyWriter` goroutine appends records to an owner-only file. It batches records queued during a write into one `fsync` and prints "saved" only after the flush. If a save fails it falls back to printing records in full, and `main` stops the search. On exit `main` drains `found` and calls `writer.close()` so queued keys are not lost.
- `search.go`: `parseSearch` validates patterns (base32, ≤58 chars, `exact` needs 58, `ends`/`exact` must end in `AEIMQUY4`), and `search.match` is the reference matcher.
- `cpu.go`: one goroutine per core calling `crypto.GenerateAccount()`.
- `gpuconfig.go` (untagged, so the flag is validated on every platform): `gpuConfig` and `parseGPUConfig`/`String` for the `window/interleave/batch[/subgroup]` syntax shared by `-gpu-config` and `BenchmarkGPU`'s `-gpu.configs`.
- `vulkan.go`: GPU backend, build-tagged to windows/linux/darwin on amd64/arm64 (`vulkan_stub.go` covers the rest). `gpuConfigFor` picks the shader variant per vendor (AMD with subgroup size control: 8/1/8/32; other AMD: 8/1/3; everyone else: 8/1/2; tuned on an RX 7900 XT only, results in the README), and `-gpu-config` overrides it. `newGPUSearcher` uploads `baseTable(w, t)` and compiles the embedded `shaders/cruncher.spv` with that variant. `gpuSearcher.search` is the dispatch loop: `dispatchSize` starts at about 2^18 candidates whatever the batch, grows workgroups (up to 32768) before iterations to keep dispatches near 200 ms, and every GPU hit is re-derived on the CPU and must match.
- `shaders/cruncher.comp`: per candidate, ChaCha20 seed → SHA-512 → clamped scalar → signed radix-2^W fixed-base scalar multiplication (a' = a>>3 recoded with a bias so the top carry is constant; an affine start table absorbs the first digit; T=2 adds ref10's even/odd doubling pass) → Montgomery batch inversion shared by BATCH candidates per invocation → compressed public key → SHA-512/256 checksum when the pattern can reach it → base32 matching. The field code is the ref10 port (10 int32 limbs, int64 products), so the shader requires `shaderInt64`.
- `internal/vk`: a minimal cgo-free Vulkan binding using `github.com/ebitengine/purego`. It loads the loader at runtime, so `go build` needs no C compiler or Vulkan SDK. Structs mirror the 64-bit C ABI by hand, and `layout_test.go`/`layout_ext_test.go` pin their sizes and offsets (values measured with gcc against `vulkan_core.h`). The instance targets Vulkan 1.3 when the loader supports 1.1+, and `DeviceOptions`/`KernelOptions` opt into a required subgroup size and pipeline statistics (`Kernel.Executables`).

## Host/shader contracts

These must change together:

- Candidate `idx` maps to seed = first 32 bytes of ChaCha20(key, counter=lo32(idx), nonce=hi32(idx)‖nonce8). `gpuSeed` in `vulkan.go` mirrors the shader, and the CPU uses it to rebuild hits from `idx`.
- Specialization constants: 0 = `gpuWorkgroupSize`, 1 = W (`gpuConfig.window`), 2 = T (`interleave`), 3 = BATCH (`batch`). A dispatch checks `g.candidates(size)` = groups × 64 × iterations × batch candidates; candidate k of loop iteration it has offset (it×BATCH + k) × stride + gid. `run`, `search`, `startSize`, the tests and the benchmark must all count with `candidates()`.
- `baseTable(w, t)` must produce exactly what `load_entry`/`ge_scalarmult_base` read: 2^W affine (x, y, xy) start entries, then ceil(m/t) rows of 2^(W-1)+1 precomputed entries (entry 0 is the identity), each 32 int32 = 128 bytes. `TestBaseTable` replays the shader's digit logic on the CPU for every W and T.
- The `Params` buffer layout (the `params*` offsets in `vulkan.go`), the `Results` layout (hit count may exceed `max_hits`; only the first `max_hits` indices are stored), push constants, and the search-type numbers (`searchExact`..`searchContains` in `search.go`). Address symbols 0..50 depend only on the public key; `starts` patterns longer than 51 characters need the checksum.
- After editing `cruncher.comp`, run `go generate` and commit the new `.spv`. The tests run the embedded `.spv`, not the source. `TestGPUDerivesAddresses` compares GPU addresses byte-for-byte with Go's ed25519 for the shipped variants plus a T=2 one; `TestGPUSearch`, `TestGPUHitOverflow` and `TestGPUSearchLoop` cover matching, the hit-buffer bound and the dispatch loop's accounting.

Printed private keys and mnemonics are real secrets; treat captured program output accordingly.
