package main

/*
// 10.13 is the floor every wails/v3 darwin file declares, and this states the same one
// for ours. Left unset, this file's objects are compiled against the host SDK while the
// link target is clamped to arm64's 11.0, and each mismatched object costs an `ld:
// warning: object file ... was built for newer 'macOS' version` line.
//
// It fixes this file's three objects and no more. Go compiles its own cgo support objects
// with whatever CGO_CFLAGS says, which no `#cgo` directive can reach — so `wails3 task
// build` is silent (build/darwin/Taskfile.yml sets the same floor build-wide, which is
// the whole of that fix) while a bare `go build` still prints seventeen. Worth knowing
// before going looking: nothing is wrong with the tree when it does.
#cgo CFLAGS: -mmacosx-version-min=10.13 -x objective-c
#cgo LDFLAGS: -framework Cocoa -mmacosx-version-min=10.13
#import <Cocoa/Cocoa.h>

typedef struct {
	double controlsCentreY;
	double controlsInlineEnd;
	int measured;
} ChromeMetrics;

static ChromeMetrics measureOnMain(void *windowHandle) {
	ChromeMetrics metrics = {0, 0, 0};
	NSWindow *window = (NSWindow *)windowHandle;
	if (window == nil) return metrics;
	NSButton *close = [window standardWindowButton:NSWindowCloseButton];
	NSButton *zoom = [window standardWindowButton:NSWindowZoomButton];
	if (close == nil || zoom == nil) return metrics;

	// Fullscreen takes the marks away with the menu bar, and a gutter reserved for
	// something that is not there is a hole in the header. Zero says "no gutter",
	// and leaves the center at zero with it so nothing is aligned to a ghost.
	if (!close.isHidden && !zoom.isHidden) {
		NSRect closeFrame = [close.superview convertRect:close.frame toView:nil];
		NSRect zoomFrame = [zoom.superview convertRect:zoom.frame toView:nil];
		metrics.controlsInlineEnd = NSMaxX(zoomFrame);
		// Window coordinates are bottom-left; CSS wants the distance down from the top.
		metrics.controlsCentreY = window.frame.size.height - NSMidY(closeFrame);
	}
	metrics.measured = 1;
	return metrics;
}

// AppKit may only be touched from the main thread. Wails runs a binding call on
// its own goroutine, so the work hops across; dispatch_sync from the main thread
// onto its own queue would deadlock, hence the check.
static ChromeMetrics windowChromeMetrics(void *windowHandle) {
	if ([NSThread isMainThread]) return measureOnMain(windowHandle);
	__block ChromeMetrics metrics;
	dispatch_sync(dispatch_get_main_queue(), ^{ metrics = measureOnMain(windowHandle); });
	return metrics;
}

*/
import "C"

import "unsafe"

// nativeWindowChrome reports where the platform put the window's own controls, so
// the header the app draws around them can be laid out against the real numbers
// rather than against remembered ones.
//
// Web content cannot see AppKit's control geometry, and hiding the native buttons
// would discard platform behavior such as the zoom button's tiling menu. Two
// numbers cross the boundary, and they are the two the app cannot otherwise know:
// where the cluster ends, which is where the header's first control may begin, and
// the marks' center line, which is what that control centers on.
//
// Deliberately NOT the titlebar height, which would be the app's header height by
// another name — that number belongs to the visual style, and handing the frame a
// second claim on it would leave one value with two owners.
//
// The exact Wails window handle is passed in; inspecting an application-global
// window list could select a sheet or file dialog. A nil handle means no window yet,
// which is valid before creation and after destruction.
func nativeWindowChrome(window unsafe.Pointer) (controlsCentreY, controlsInlineEnd float64, measured bool) {
	metrics := C.windowChromeMetrics(window)
	return float64(metrics.controlsCentreY), float64(metrics.controlsInlineEnd), metrics.measured != 0
}
