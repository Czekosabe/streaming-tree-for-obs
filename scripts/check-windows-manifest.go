//go:build ignore

// check-windows-manifest extracts and prints the real RT_MANIFEST
// resource (Win32 resource type 24) embedded in a built Windows .exe,
// via the same FindResource/LoadResource/LockResource sequence
// documented at learn.microsoft.com/windows/win32/menurc/resource-types
// - never by reading source, never by trusting the winres.json input
// alone. Exits nonzero, printing nothing to stdout, if no manifest
// resource (id 1 or 2) is present at all.
//
// Used by scripts/verify-packaged-app.mjs (Stage 20E DPI-awareness
// remediation) to prove, on real Windows CI, that the shipped
// executable actually carries the Per-Monitor-V2 DPI-awareness
// manifest apps/server/cmd/server/winres/winres.json declares - not
// just that the source config says so.
//
// Usage: go run scripts/check-windows-manifest.go <path-to-exe>
package main

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

var (
	modKernel32        = syscall.NewLazyDLL("kernel32.dll")
	procLoadLibraryExW = modKernel32.NewProc("LoadLibraryExW")
	procFindResourceW  = modKernel32.NewProc("FindResourceW")
	procSizeofResource = modKernel32.NewProc("SizeofResource")
	procLoadResource   = modKernel32.NewProc("LoadResource")
	procLockResource   = modKernel32.NewProc("LockResource")
	procFreeLibrary    = modKernel32.NewProc("FreeLibrary")
)

const (
	loadLibraryAsDatafile = 0x00000002
	rtManifest            = 24
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: check-windows-manifest <path-to-exe>")
		os.Exit(2)
	}
	path := os.Args[1]

	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "encode path:", err)
		os.Exit(1)
	}
	h, _, callErr := procLoadLibraryExW.Call(uintptr(unsafe.Pointer(pathPtr)), 0, loadLibraryAsDatafile)
	if h == 0 {
		fmt.Fprintln(os.Stderr, "LoadLibraryExW failed:", callErr)
		os.Exit(1)
	}
	defer procFreeLibrary.Call(h)

	// Manifests conventionally use resource ID 1 (CREATEPROCESS_MANIFEST_
	// RESOURCE_ID); ID 2 is the DLL convention, checked too for
	// completeness even though this tool only ever targets an .exe.
	for _, id := range []uintptr{1, 2} {
		hRes, _, _ := procFindResourceW.Call(h, id, rtManifest)
		if hRes == 0 {
			continue
		}
		size, _, _ := procSizeofResource.Call(h, hRes)
		hData, _, _ := procLoadResource.Call(h, hRes)
		ptr, _, _ := procLockResource.Call(hData)
		if ptr == 0 || size == 0 {
			continue
		}
		data := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), int(size))
		fmt.Print(string(data))
		return
	}

	fmt.Fprintln(os.Stderr, "no RT_MANIFEST resource found (id 1 or 2)")
	os.Exit(1)
}
