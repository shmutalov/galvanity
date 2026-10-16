//go:build windows && (amd64 || arm64)

package vk

import "syscall"

func openLibrary() (uintptr, error) {
	h, err := syscall.LoadLibrary("vulkan-1.dll")
	return uintptr(h), err
}

func symbol(lib uintptr, name string) (uintptr, error) {
	return syscall.GetProcAddress(syscall.Handle(lib), name)
}
