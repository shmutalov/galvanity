package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/algorand/go-algorand-sdk/crypto"
)

func TestKeyFile(t *testing.T) {
	name := filepath.Join(t.TempDir(), "found.txt")
	a, b := crypto.GenerateAccount(), crypto.GenerateAccount()

	// Two sessions: records are appended, never overwritten.
	for _, acc := range []crypto.Account{a, b} {
		keys, err := openKeyFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := keys.save(acc); err != nil {
			t.Fatal(err)
		}
		keys.f.Close()
	}

	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if want := accountRecord(a) + accountRecord(b); string(data) != want {
		t.Errorf("file contains:\n%s\nwant:\n%s", data, want)
	}
	if !strings.Contains(string(data), "MNEMONIC: ") {
		t.Error("record has no mnemonic")
	}

	if runtime.GOOS != "windows" {
		fi, err := os.Stat(name)
		if err != nil {
			t.Fatal(err)
		}
		if perm := fi.Mode().Perm(); perm != 0o600 {
			t.Errorf("key file mode = %v, want -rw-------", perm)
		}
	}
}

// captureStdout redirects os.Stdout to a file for the rest of the test and
// returns a function that reads what was written so far.
func captureStdout(t *testing.T) func() string {
	t.Helper()
	f, err := os.Create(filepath.Join(t.TempDir(), "stdout.txt"))
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = f
	t.Cleanup(func() { os.Stdout = old; f.Close() })
	return func() string {
		data, err := os.ReadFile(f.Name())
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
}

func TestKeyWriter(t *testing.T) {
	stdout := captureStdout(t)
	name := filepath.Join(t.TempDir(), "found.txt")
	keys, err := openKeyFile(name)
	if err != nil {
		t.Fatal(err)
	}
	defer keys.f.Close()

	w := newKeyWriter(keys, name)
	var want strings.Builder
	accounts := make([]crypto.Account, 100)
	for i := range accounts {
		accounts[i] = crypto.GenerateAccount()
		want.WriteString(accountRecord(accounts[i]))
		w.add(accounts[i])
	}
	w.close()

	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want.String() {
		t.Error("key file does not hold every record in order")
	}
	out := stdout()
	if strings.Contains(out, "MNEMONIC") || strings.Contains(out, "PK:") {
		t.Error("secrets were printed to stdout")
	}
	for _, a := range accounts {
		if !strings.Contains(out, "Found ADDR: "+a.Address.String()+" (saved to ") {
			t.Errorf("%s not reported as saved", a.Address)
		}
	}
	select {
	case err := <-w.failed:
		t.Errorf("unexpected save error: %v", err)
	default:
	}
}

func TestKeyWriterFailure(t *testing.T) {
	stdout := captureStdout(t)
	name := filepath.Join(t.TempDir(), "found.txt")
	keys, err := openKeyFile(name)
	if err != nil {
		t.Fatal(err)
	}
	keys.f.Close() // every save fails

	w := newKeyWriter(keys, name)
	a, b := crypto.GenerateAccount(), crypto.GenerateAccount()
	w.add(a)
	w.add(b)
	w.close()

	if err := <-w.failed; err == nil {
		t.Error("no error reported")
	}
	// Keys that could not be saved must still be shown in full.
	out := stdout()
	for _, acc := range []crypto.Account{a, b} {
		if !strings.Contains(out, accountRecord(acc)) {
			t.Errorf("%s was neither saved nor printed", acc.Address)
		}
	}
}
