package main

import (
	"errors"
	"fmt"
	"strings"
)

type searchType int

// The numeric values are shared with shaders/cruncher.comp.
const (
	searchExact searchType = iota
	searchStarts
	searchEnds
	searchContains
)

var searchTypes = map[string]searchType{
	"exact":    searchExact,
	"starts":   searchStarts,
	"ends":     searchEnds,
	"contains": searchContains,
}

func (t searchType) String() string {
	for name, v := range searchTypes {
		if v == t {
			return name
		}
	}
	return fmt.Sprintf("searchType(%d)", int(t))
}

const (
	// addressLen is the length of an Algorand address: base32 of a 32-byte
	// public key plus a 4-byte checksum, without padding.
	addressLen     = 58
	base32Alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
	// The last address character carries only 3 bits of data followed by 2
	// zero bits, so it is always one of these.
	lastCharAlphabet = "AEIMQUY4"
)

type search struct {
	typ     searchType
	pattern string
}

// parseSearch parses the positional arguments: [search-type] <pattern>.
func parseSearch(args []string) (search, error) {
	var typName, pattern string
	switch len(args) {
	case 0:
		return search{}, errors.New("Not enough arguments")
	case 1:
		typName, pattern = "contains", args[0]
	case 2:
		typName, pattern = args[0], args[1]
	default:
		return search{}, errors.New("Too many arguments")
	}

	pattern = strings.ToUpper(pattern)
	if pattern == "" || strings.Trim(pattern, base32Alphabet) != "" {
		return search{}, errors.New("Invalid pattern, must match to the BASE32 format: only A-Z and 2-7 (excluding 0/zero, 1/one, 8/eight and 9/nine)")
	}
	typ, ok := searchTypes[strings.ToLower(typName)]
	if !ok {
		return search{}, errors.New("Invalid search type")
	}

	if len(pattern) > addressLen {
		return search{}, fmt.Errorf("Invalid pattern, an address is only %d characters long", addressLen)
	}
	if typ == searchExact && len(pattern) != addressLen {
		return search{}, fmt.Errorf("Invalid pattern, exact search needs a full %d-character address", addressLen)
	}
	if (typ == searchEnds || typ == searchExact) && !strings.ContainsRune(lastCharAlphabet, rune(pattern[len(pattern)-1])) {
		return search{}, fmt.Errorf("Invalid pattern, an address always ends with one of %s", strings.Join(strings.Split(lastCharAlphabet, ""), ", "))
	}
	return search{typ: typ, pattern: pattern}, nil
}

func (s search) match(address string) bool {
	switch s.typ {
	case searchExact:
		return address == s.pattern
	case searchStarts:
		return strings.HasPrefix(address, s.pattern)
	case searchEnds:
		return strings.HasSuffix(address, s.pattern)
	default:
		return strings.Contains(address, s.pattern)
	}
}
