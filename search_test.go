package main

import (
	"strings"
	"testing"

	"github.com/algorand/go-algorand-sdk/crypto"
)

func TestParseSearch(t *testing.T) {
	full := "ALGO2INGB7XUD3TCMEUELGPDTKXWSGBFIQSIRVN66NI3WMCSLSUNBST6NM"
	valid := []struct {
		args []string
		want search
	}{
		{[]string{"algo"}, search{searchContains, "ALGO"}},
		{[]string{"starts", "ALGO"}, search{searchStarts, "ALGO"}},
		{[]string{"ENDS", "a7q"}, search{searchEnds, "A7Q"}},
		{[]string{"contains", "Z2"}, search{searchContains, "Z2"}},
		{[]string{"exact", strings.ToLower(full)}, search{searchExact, full}},
	}
	for _, c := range valid {
		got, err := parseSearch(c.args)
		if err != nil || got != c.want {
			t.Errorf("parseSearch(%q) = %v, %v; want %v", c.args, got, err, c.want)
		}
	}

	invalid := [][]string{
		nil,
		{"starts", "ALGO", "X"},
		{"starts", ""},
		{"starts", "ALG0"}, // zero is not base32
		{"begins", "ALGO"}, // unknown search type
		{"ends", "ALGOB"},  // impossible last character
		{"exact", "ALGO"},  // not a full address
		{"exact", full[:57] + "B"},
		{"starts", strings.Repeat("A", addressLen+1)},
	}
	for _, args := range invalid {
		if got, err := parseSearch(args); err == nil {
			t.Errorf("parseSearch(%q) = %v, want an error", args, got)
		}
	}
}

func TestAddressLastChar(t *testing.T) {
	for range 200 {
		addr := crypto.GenerateAccount().Address.String()
		if len(addr) != addressLen || !strings.ContainsRune(lastCharAlphabet, rune(addr[len(addr)-1])) {
			t.Fatalf("address %s breaks the length/last-character assumptions", addr)
		}
	}
}

func TestSearchMatch(t *testing.T) {
	addr := "ALGO2INGB7XUD3TCMEUELGPDTKXWSGBFIQSIRVN66NI3WMCSLSUNBST6NM"
	cases := []struct {
		s    search
		want bool
	}{
		{search{searchStarts, "ALGO"}, true},
		{search{searchStarts, "LGO"}, false},
		{search{searchEnds, "T6NM"}, true},
		{search{searchEnds, "ALGO"}, false},
		{search{searchContains, "SIRV"}, true},
		{search{searchContains, "ZZZZ"}, false},
		{search{searchExact, addr}, true},
		{search{searchExact, addr[:57] + "A"}, false},
	}
	for _, c := range cases {
		if got := c.s.match(addr); got != c.want {
			t.Errorf("%v.match(%s) = %v, want %v", c.s, addr, got, c.want)
		}
	}
}
