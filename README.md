[![build](https://github.com/shmutalov/galvanity/actions/workflows/build.yml/badge.svg)](https://github.com/shmutalov/galvanity/actions/workflows/build.yml)

# Galvanity

Galvanity is Algorand vanity address generator written in Go

# Usage

`galvanity [flags] [search-type] <pattern>`

`search-type` is matching function to search for the pattern, it can be:
 - `exact`    - search exact pattern (full address string)
 - `starts`   - search address which starts with given pattern
 - `ends`     - search address which ends with given pattern
 - `contains` - search address which contains given pattern at any place (default)

`pattern` is correct [**BASE32** / RFC 4648](https://datatracker.ietf.org/doc/html/rfc4648) hash string, alphabet is `A-Z` and `2-7`.
An address is 58 characters long and its last character is always one of `A`, `E`, `I`, `M`, `Q`, `U`, `Y`, `4`,
so `ends` and `exact` patterns must end with one of them.

Flags (they must come before `search-type`):
 - `-backend cpu|vulkan` - search on all CPU cores (default) or on a GPU via Vulkan
 - `-device N`           - Vulkan device to use (default: the first usable discrete GPU, else integrated, virtual or CPU
   device; `-list-devices` marks it `default`)
 - `-list-devices`       - list Vulkan devices and exit
 - `-out FILE`           - append found keys to `FILE` instead of printing them; only addresses are printed
 - `-gpu-config W/T/K[/S]` - override the GPU settings, see [GPU settings](#gpu-settings)

`PK` and `MNEMONIC` in the output are secrets: either one is enough to spend from the address. By default they are
printed to the terminal, where scrollback, logs or redirected output can keep them. With `-out found.txt` they are
appended to `found.txt` (created readable only by you on Linux/macOS; on Windows it inherits the folder's
permissions), each record is flushed to disk before its address is shown, and the terminal shows only
`Found ADDR: ... (saved to found.txt)`.

# GPU search (Vulkan)

`galvanity -backend vulkan starts ALGO` runs the search on a GPU. It works on any AMD, NVIDIA or Intel GPU
whose Vulkan driver supports 64-bit integers in shaders (`shaderInt64`, which desktop GPU drivers generally do;
`-list-devices` marks devices that cannot be used).
On Windows and Linux only the GPU driver is needed, not the Vulkan SDK. macOS has no native Vulkan: install MoltenVK
and the Vulkan loader (`brew install molten-vk vulkan-loader`, or the LunarG Vulkan SDK); this path is untested.
The first run compiles the shader, which can take several seconds; the driver caches it after that.

The GPU derives each candidate key from a random secret generated for every run (ChaCha20), so found keys are as
unpredictable as the CPU ones and unrelated to each other. Every match the GPU reports is derived again on the CPU and
checked before it is printed.

## GPU settings

The shader has four settings, written `window/interleave/batch[/subgroup]`:

 - `window` (4..8) - bits per signed digit of the private scalar. Bigger windows need fewer point additions but a larger
   precomputed table (548 KB for 8).
 - `interleave` (1 or 2) - 1 uses a table row per digit; 2 halves the table and adds `window` point doublings.
 - `batch` (1..16) - keys per GPU thread that share one field inversion. A bigger batch does less work per key but
   needs more registers.
 - `subgroup` (optional) - a required subgroup (wave) size, a power of two the GPU supports; by default the driver
   chooses.

The defaults depend on the GPU: `8/1/8/32` for AMD GPUs that can run wave32 (RDNA), `8/1/3` for other AMD GPUs, and
`8/1/2` for everything else, because NVIDIA and Intel GPUs have not been measured yet. The settings in use are printed
at startup, and `-list-devices` shows each device's defaults. To try others, pass `-gpu-config`:

```
galvanity -backend vulkan -gpu-config 8/1/4 starts ALGOXYZW
```

To tune for your GPU, run a `starts` search for 8 or more characters, which will not be found during the test (about
one hit per 3 hours at 89 MH/s), with different settings, and compare the `Speed` lines once they settle (the first
includes compiling the shader). `contains` and `ends` searches also hash each candidate's checksum on the GPU, so they
run slower than the numbers below, which were measured with `starts ALGOXYZ`.
From the source tree, `go test -run '^$' -bench GPU -benchtime 20x -gpu.stats -gpu.configs 8/1/1,8/1/2,8/1/4,8/1/8`
measures several settings in one go and also prints register and scratch memory use where the driver reports them.

Tuning results on an AMD Radeon RX 7900 XT (driver 26.8.1), in million candidates per second. Window sizes, with one
key per thread at the driver's wave size (measured with smaller dispatches than the batch table, so about 2 lower):

| Settings | 4/2 | 6/1 | 7/2 | 8/2 | 7/1 | 8/1 |
|----------|-----|-----|-----|-----|-----|-----|
| Mcand/s  | 35  | 46  | 46  | 49  | 51  | 54  |

Batch sizes with `8/1`. Registers are per GPU thread (256 is the maximum); spilled is the scratch memory per thread
the compiler used for values that did not fit, at wave32. Empty cells were not measured:

| Batch | Registers | Spilled | wave64 (driver default) | wave32 (`/32`) |
|-------|-----------|---------|-------------------------|----------------|
| 1     | 141       | 0       | 56                      |                |
| 2     | 182       | 0       | 70                      | 71             |
| 3     | 228       | 0       | 78                      | 79             |
| 4     | 256       | 32 B    | 78                      | 84             |
| 5     | 256       | 192 B   |                         | 86             |
| 6     | 256       | 352 B   | 79                      | 87             |
| 8     | 256       | 704 B   | 77                      | 89             |
| 12    | 256       | 1936 B  |                         | 84             |
| 16    | 256       | 2576 B  |                         | 82             |

# Examples

`galvanity starts ALGO` - program will try to find generated addresses which starts with `ALGO` in their name

Output will be like:

```
$ ./galvanity starts ALGO
Pattern to find: ALGO
Search type: starts
Backend: cpu
Matching started...
Processed: 1 MH Speed: 182.17 KH/s Time elapsed: 5.49 s
Processed: 2 MH Speed: 181.41 KH/s Time elapsed: 11.00 s

==== ==== ====
Found ADDR: ALGO2INGB7XUD3TCMEUELGPDTKXWSGBFIQSIRVN66NI3WMCSLSUNBST6NM
PUB: [2 204 237 33 166 15 239 65 238 98 97 40 69 153 227 154 175 105 24 37 68 36 136 213 190 243 81 187 48 82 92 168]
PK: [... 64 bytes, secret ...]
MNEMONIC: <25 secret words>
==== ==== ====

Processed: 3 MH Speed: 181.87 KH/s Time elapsed: 16.50 s
Processed: 4 MH Speed: 181.97 KH/s Time elapsed: 22.00 s
Processed: 5 MH Speed: 182.19 KH/s Time elapsed: 27.48 s

==== ==== ====
Found ADDR: ALGOFQER73JZZEOBUOS37WNFF2X7EFISER2XWLHVBU2QWFLI4BZTA7TDNE
PUB: [2 204 226 192 145 254 211 156 145 193 163 165 191 217 165 46 175 242 21 18 36 117 123 44 245 13 53 11 21 104 224 115]
PK: [... 64 bytes, secret ...]
MNEMONIC: <25 secret words>
==== ==== ====

Processed: 6 MH Speed: 181.98 KH/s Time elapsed: 32.98 s
Processed: 7 MH Speed: 182.55 KH/s Time elapsed: 38.46 s
Processed: 8 MH Speed: 181.96 KH/s Time elapsed: 43.95 s
Processed: 9 MH Speed: 182.45 KH/s Time elapsed: 49.43 s

==== ==== ====
Found ADDR: ALGOYTE6VC4PAAKHDO4IKCKZB3L5KET7T4S6V5747AQGLS4NPU6D7APLBQ
PUB: [2 204 236 76 158 168 184 240 1 71 27 184 133 9 89 14 215 213 18 127 159 37 234 247 252 248 32 101 203 141 125 60]
PK: [... 64 bytes, secret ...]
MNEMONIC: <25 secret words>
==== ==== ====


==== ==== ====
Found ADDR: ALGOQL4QIQJQCEENXABSWUY7UEKFJQFP72HL7GGNQVE4P4URBD5AIXTH7M
PUB: [2 204 232 47 144 68 19 1 16 141 184 3 43 83 31 161 20 84 192 175 254 142 191 152 205 133 73 199 242 145 8 250]
PK: [... 64 bytes, secret ...]
MNEMONIC: <25 secret words>
==== ==== ====

```

As you see, found addresses are:
1. `ALGO2INGB7XUD3TCMEUELGPDTKXWSGBFIQSIRVN66NI3WMCSLSUNBST6NM`
2. `ALGOFQER73JZZEOBUOS37WNFF2X7EFISER2XWLHVBU2QWFLI4BZTA7TDNE`
3. `ALGOYTE6VC4PAAKHDO4IKCKZB3L5KET7T4S6V5747AQGLS4NPU6D7APLBQ`
4. `ALGOQL4QIQJQCEENXABSWUY7UEKFJQFP72HL7GGNQVE4P4URBD5AIXTH7M`

It is infinity process, to stop you may send CTRL+C to the program.

Speed is `~180 000 address checks per second` on my `AMD Ryzen 7 2700x`.

Measured with `starts ALGOXYZ`:

| Backend  | Hardware              | Speed       |
|----------|-----------------------|-------------|
| `cpu`    | Intel Core i5-12400F  | ~0.37 MH/s  |
| `vulkan` | AMD Radeon RX 7900 XT | ~89 MH/s    |

# Build

To build the Galvanity, you need to download and install the Go compiler, version 1.27.2 or newer.
No C compiler or Vulkan SDK is needed.

Then just run the following command in the source code directory:

```
go build
```

After successful build, it will produce `galvanity` (or `galvanity.exe` on Windows OS) named binary file

The compiled GPU shader (`shaders/cruncher.spv`) is committed. After changing `shaders/cruncher.comp`, rebuild it with
`go generate`, which needs `glslc` from the [Vulkan SDK](https://vulkan.lunarg.com/).

`go test ./...` runs the GPU tests on the default Vulkan device and skips them when there is none
(`GALVANITY_REQUIRE_VULKAN=1` makes that a failure, as CI does on Linux, where it installs Mesa's lavapipe).

# Pre-built binaries

Download a release for Windows, Linux or macOS (amd64 and arm64) from the [**Releases**](https://github.com/shmutalov/galvanity/releases) page; `SHA256SUMS` lists the checksums of its files.

Builds of every commit are on the [**build-action**](https://github.com/shmutalov/galvanity/actions/workflows/build.yml) page: select the latest successful build, scroll down and download the needed artifacts.

# Roadmap

- [ ] Add the `threads` parameter
- [x] Implement the GPU-side generator (Vulkan)

# Donations

I accept donations to `DONATEYMDVCMPY2KKHTCTR2OAKLFQAJNGXRDXMJ7GJYXEZCHCNCHF4JABE`

# License

Galvanity is distributed under the terms GNU General Public License (Version 3).

See [LICENSE](./LICENSE) for details.