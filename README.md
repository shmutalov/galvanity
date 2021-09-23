[![build](https://github.com/shmutalov/galvanity/actions/workflows/build.yml/badge.svg)](https://github.com/shmutalov/galvanity/actions/workflows/build.yml)

# Galvanity

Galvanity is Algorand vanity address generator written in Go

# Usage

`galvanity [search-type] <pattern>`

`search-type` is matching function to search for the pattern, it can be:
 - `exact`    - search exact pattern (full address string)
 - `starts`   - search address which starts with given pattern
 - `ends`     - search address which ends with given pattern
 - `contains` - search address which contains given pattern at any place

`pattern` is correct [**BASE32** / RFC 4648](https://datatracker.ietf.org/doc/html/rfc4648) hash string, alphabet is `A-Z` and `2-7`

# Examples

`galvanity starts AAAA` - program will try to find generated addresses which starts with 'AAAA' in their name

Output will be like:

```
$ ./galvanity starts AAAA
Pattern to find: AAAA
Search type: starts
Matching started...
Processed: 6 MH Speed: 7.90 MH/s Time elapsed: 0.76 s
Processed: 197 MH Speed: 52.62 MH/s Time elapsed: 4.39 s
Processed: 830 MH Speed: 136.04 MH/s Time elapsed: 9.04 s
Processed: 1136 MH Speed: 190.71 MH/s Time elapsed: 10.65 s
Processed: 2085 MH Speed: 240.54 MH/s Time elapsed: 14.59 s
Processed: 2087 MH Speed: 270.46 MH/s Time elapsed: 14.60 s
Processed: 3255 MH Speed: 309.62 MH/s Time elapsed: 18.37 s

==== ==== ====
Found ADDR: AAAAPPFC7633QS2CPPCNZXXF2E3TG6ZUPBZHP3WCINWGITNRFZ5SNHUCJ4
PUB: [0 0 7 188 162 255 183 184 75 66 123 196 220 222 229 209 55 51 123 52 120 114 119 238 194 67 108 100 77 177 46 123]
PK: [234 229 118 78 193 186 11 44 186 238 108 36 188 5 21 188 254 36 156 60 72 18 83 133 163 120 47 101 245 31 100 253 0 0 7 188 162 255 183 184 75 66 123 196 220 222 229 209 55 51 123 52 120 114 119 238 194 67 108 100 77 177 46 123]
MNEMONIC: run swear poet project blast mention into hollow loyal black appear type enemy check draft banner price mix rough fancy turn among twenty abstract sunny
==== ==== ====


==== ==== ====
Found ADDR: AAAASJVCK5SKIOOVSATN35KVATJECT65CFZNX5P3TLAILICIPD5N6HEWFM
PUB: [0 0 9 38 162 87 100 164 57 213 144 38 221 245 85 4 210 65 79 221 17 114 219 245 251 154 192 133 160 72 120 250]
PK: [161 219 67 127 134 21 39 226 121 225 196 130 68 148 80 18 132 10 30 14 135 141 229 105 143 141 83 234 95 153 29 168 0 0 9 38 162 87 100 164 57 213 144 38 221 245 85 4 210 65 79 221 17 114 219 245 251 154 192 133 160 72 120 250]
MNEMONIC: injury author distance flash evolve joy armed shaft motion extra choose donkey bench maple debris mirror device diet short pioneer save green domain absent era
==== ==== ====


```

# Build

To build the Galvanity, you need to download and install the Go compiler (I tested with 1.15 version).

Then just run the following command in the source code directory:

```
go build
```

After successful build, it will produce `galvanity` (or `galvanity.exe` on Windows OS) named binary file

# Pre-built binaries

Check pre-built binaries at [**build-action**](https://github.com/shmutalov/galvanity/actions/workflows/build.yml) page, select the latest successful build, scroll down and download the needed artifacts.

# TODO

- [ ] Add the `threads` parameter
- [ ] Implement the GPU-side generator

# License

Galvanity is distributed under the terms GNU General Public License (Version 3).

See [LICENSE](./LICENSE) for details.