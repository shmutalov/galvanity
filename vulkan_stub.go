//go:build !((windows || linux || darwin) && (amd64 || arm64))

package main

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/algorand/go-algorand-sdk/crypto"
)

var errNoVulkan = errors.New("the Vulkan backend is not available on this platform")

type vulkanBackend struct {
	device int
	config *gpuConfig
}

func (vulkanBackend) run(context.Context, search, chan<- crypto.Account, *atomic.Uint64) error {
	return errNoVulkan
}

func printVulkanDevices() error { return errNoVulkan }
