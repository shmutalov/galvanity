package main

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/algorand/go-algorand-sdk/crypto"
)

// cpuBackend generates accounts with crypto/rand on every CPU core.
type cpuBackend struct{}

func (cpuBackend) run(ctx context.Context, s search, found chan<- crypto.Account, checked *atomic.Uint64) error {
	const batch = 1024
	var wg sync.WaitGroup
	for range runtime.NumCPU() {
		wg.Go(func() {
			for ctx.Err() == nil {
				for range batch {
					account := crypto.GenerateAccount()
					if s.match(account.Address.String()) {
						found <- account // main reads found until run returns
					}
				}
				checked.Add(batch)
			}
		})
	}
	wg.Wait()
	return nil
}
