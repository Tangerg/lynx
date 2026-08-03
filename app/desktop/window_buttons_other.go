//go:build !darwin

package main

// hideNativeWindowButtons is a macOS concern: it is the only platform whose
// window frame carries controls the app wants to draw for itself.
func hideNativeWindowButtons() {}
