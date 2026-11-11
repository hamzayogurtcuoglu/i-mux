//go:build windows

package sysx

import "unsafe"

func uintptrOf(r *inputRecord) uintptr { return uintptr(unsafe.Pointer(r)) }
func uintptrOfU32(p *uint32) uintptr   { return uintptr(unsafe.Pointer(p)) }
