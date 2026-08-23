//go:build darwin

package main

/*
#cgo CFLAGS: -mmacosx-version-min=10.13 -x objective-c
#cgo LDFLAGS: -framework Cocoa -mmacosx-version-min=10.13
#import <Cocoa/Cocoa.h>

typedef struct {
	double centreY;
	double inlineEnd;
	int measured;
} LyraChromeMetrics;

static LyraChromeMetrics lyraMeasureChrome(void *handle) {
	LyraChromeMetrics result = {0, 0, 0};
	NSWindow *window = (NSWindow *)handle;
	if (window == nil) return result;
	NSButton *close = [window standardWindowButton:NSWindowCloseButton];
	NSButton *zoom = [window standardWindowButton:NSWindowZoomButton];
	if (close == nil || zoom == nil) return result;
	if (!close.isHidden && !zoom.isHidden) {
		NSRect closeFrame = [close.superview convertRect:close.frame toView:nil];
		NSRect zoomFrame = [zoom.superview convertRect:zoom.frame toView:nil];
		result.centreY = window.frame.size.height - NSMidY(closeFrame);
		result.inlineEnd = NSMaxX(zoomFrame);
	}
	result.measured = 1;
	return result;
}

static LyraChromeMetrics lyraWindowChrome(void *handle) {
	if ([NSThread isMainThread]) return lyraMeasureChrome(handle);
	__block LyraChromeMetrics result;
	dispatch_sync(dispatch_get_main_queue(), ^{ result = lyraMeasureChrome(handle); });
	return result;
}
*/
import "C"

import "unsafe"

func nativeWindowChrome(window unsafe.Pointer) (float64, float64, bool) {
	metrics := C.lyraWindowChrome(window)
	return float64(metrics.centreY), float64(metrics.inlineEnd), metrics.measured != 0
}
