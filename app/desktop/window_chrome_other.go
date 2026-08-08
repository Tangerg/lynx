//go:build !darwin

package main

import "unsafe"

// Window chrome is a macOS concern: it is the only platform whose frame puts its
// controls over the app's own content, where a header has to be laid out around
// them.
//
// `measured` false leaves the frontend on the stylesheet's own header height and
// gutter, which is what every other platform wants — the controls are outside the
// content there, so nothing has to be reserved for them. The window handle is taken
// and ignored for the same reason: there is nothing on it to measure.

func nativeWindowChrome(unsafe.Pointer) (controlsCentreY, controlsInlineEnd float64, measured bool) {
	return 0, 0, false
}
