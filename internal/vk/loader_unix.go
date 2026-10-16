//go:build (linux || darwin) && (amd64 || arm64)

package vk

import (
	"errors"
	"runtime"

	"github.com/ebitengine/purego"
)

func openLibrary() (uintptr, error) {
	names := []string{"libvulkan.so.1", "libvulkan.so"}
	if runtime.GOOS == "darwin" {
		// The LunarG SDK / Homebrew loader first, then MoltenVK directly.
		names = []string{"libvulkan.1.dylib", "libvulkan.dylib", "libMoltenVK.dylib"}
	}
	var errs []error
	for _, name := range names {
		h, err := purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err == nil {
			return h, nil
		}
		errs = append(errs, err)
	}
	return 0, errors.Join(errs...)
}

func symbol(lib uintptr, name string) (uintptr, error) {
	return purego.Dlsym(lib, name)
}
