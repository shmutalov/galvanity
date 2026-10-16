package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/algorand/go-algorand-sdk/crypto"
	"github.com/algorand/go-algorand-sdk/mnemonic"
)

// accountRecord is the block reported for every match. PK and MNEMONIC are
// secrets: either one is enough to spend from the address.
func accountRecord(account crypto.Account) string {
	words, _ := mnemonic.FromPrivateKey(account.PrivateKey)
	return fmt.Sprintf(`
==== ==== ====
Found ADDR: %s
PUB: %v
PK: %v
MNEMONIC: %s
==== ==== ====

`, account.Address, account.PublicKey, account.PrivateKey, words)
}

func printAccount(account crypto.Account) {
	fmt.Print(accountRecord(account))
}

// keyFile appends account records to a file instead of the terminal (-out).
type keyFile struct {
	f *os.File
}

// openKeyFile opens name for appending, creating it readable only by its
// owner. On Windows the permission bits do not apply; the file inherits
// the directory's access rules.
func openKeyFile(name string) (*keyFile, error) {
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	if fi, err := f.Stat(); err == nil && runtime.GOOS != "windows" && fi.Mode().Perm()&0o077 != 0 {
		fmt.Fprintf(os.Stderr, "warning: %s is accessible by other users (mode %v)\n", name, fi.Mode().Perm())
	}
	return &keyFile{f: f}, nil
}

// save appends the accounts' records and flushes them to disk, so the keys
// survive a crash or power loss once save returns.
func (k *keyFile) save(accounts ...crypto.Account) error {
	var b strings.Builder
	for _, a := range accounts {
		b.WriteString(accountRecord(a))
	}
	if _, err := k.f.WriteString(b.String()); err != nil {
		return err
	}
	return k.f.Sync()
}

// keyWriter saves found accounts on its own goroutine, so neither the search
// nor progress output waits for the disk. Accounts that queue up during a
// save are written together and flushed with a single fsync. An address is
// reported as saved only once its record is on disk.
type keyWriter struct {
	file   *keyFile
	name   string
	queue  chan crypto.Account
	failed chan error // receives the first save error
	done   chan struct{}
}

func newKeyWriter(file *keyFile, name string) *keyWriter {
	w := &keyWriter{
		file:   file,
		name:   name,
		queue:  make(chan crypto.Account, 1024),
		failed: make(chan error, 1),
		done:   make(chan struct{}),
	}
	go w.run()
	return w
}

func (w *keyWriter) add(account crypto.Account) { w.queue <- account }

// close saves everything still queued and waits for the writer to finish.
func (w *keyWriter) close() {
	close(w.queue)
	<-w.done
}

func (w *keyWriter) run() {
	defer close(w.done)
	var err error
	for first := range w.queue {
		batch := []crypto.Account{first}
	drain:
		for {
			select {
			case next, ok := <-w.queue:
				if !ok {
					break drain
				}
				batch = append(batch, next)
			default:
				break drain
			}
		}

		if err == nil {
			if err = w.file.save(batch...); err != nil {
				w.failed <- fmt.Errorf("cannot save to %s: %w", w.name, err)
			}
		}
		for _, a := range batch {
			if err != nil {
				printAccount(a) // never lose a found key
			} else {
				fmt.Printf("Found ADDR: %s (saved to %s)\n", a.Address, w.name)
			}
		}
	}
}
