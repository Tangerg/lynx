//go:build !darwin

package main

import "unsafe"

func nativeWindowChrome(unsafe.Pointer) (float64, float64, bool) { return 0, 0, false }
