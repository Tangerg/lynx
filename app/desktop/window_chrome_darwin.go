package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

typedef struct {
	double controlsCentreY;
	double controlsInlineEnd;
	int measured;
} ChromeMetrics;

// The window Wails created. Panels and sheets opened later carry their own
// chrome and are not what the app's header has to line up with.
static NSWindow *appWindow(void) {
	for (NSWindow *window in [NSApp windows]) {
		if ([window standardWindowButton:NSWindowCloseButton] != nil) return window;
	}
	return nil;
}

// AppKit centres the three frame buttons in the titlebar, and a toolbar is what
// makes the titlebar taller. Left at NSWindowToolbarStyleAutomatic it resolves,
// on a transparent-titlebar window, to a 66pt titlebar with the marks 26pt down —
// far below where a tool window's header sits. Compact is 40pt with the marks at 20.
static void applyCompactToolbarOnMain(void) {
	NSWindow *window = appWindow();
	if (window != nil) window.toolbarStyle = NSWindowToolbarStyleUnifiedCompact;
}

static ChromeMetrics measureOnMain(void) {
	ChromeMetrics metrics = {0, 0, 0};
	NSWindow *window = appWindow();
	if (window == nil) return metrics;
	NSButton *close = [window standardWindowButton:NSWindowCloseButton];
	NSButton *zoom = [window standardWindowButton:NSWindowZoomButton];
	if (close == nil || zoom == nil) return metrics;

	// Fullscreen takes the marks away with the menu bar, and a gutter reserved for
	// something that is not there is a hole in the header. Zero says "no gutter",
	// and leaves the centre at zero with it so nothing is aligned to a ghost.
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
static ChromeMetrics windowChromeMetrics(void) {
	if ([NSThread isMainThread]) return measureOnMain();
	__block ChromeMetrics metrics;
	dispatch_sync(dispatch_get_main_queue(), ^{ metrics = measureOnMain(); });
	return metrics;
}

static void applyCompactToolbar(void) {
	if ([NSThread isMainThread]) {
		applyCompactToolbarOnMain();
		return;
	}
	dispatch_sync(dispatch_get_main_queue(), ^{ applyCompactToolbarOnMain(); });
}
*/
import "C"

// nativeWindowChrome reports where the platform put the window's own controls, so
// the header the app draws around them can be laid out against the real numbers
// rather than against remembered ones.
//
// The app used to hide the three frame buttons and draw its own, for one reason:
// nothing inside a webview can see where AppKit put them, so the gutter that
// reserves room for them was a literal. That literal was wrong. Measured on
// macOS 26 with this window's exact options, the marks centre 16pt down at
// x 16 / 39 / 62 on a 23pt pitch; the drawn ones sat 21pt down at 20 / 40 / 60 on
// a 20pt pitch — 4-5pt off the frame they were imitating, in both axes. Hiding
// them also cost the behaviours that belong to the real buttons and cannot be
// re-sent from our side: the zoom button's tiling menu (halves, quarters, fill,
// arrange, full screen, move to display) and option-click to fit rather than fill.
//
// So the buttons are the platform's again and the arithmetic is measured. Two
// numbers cross the boundary, and they are the two the app cannot otherwise know:
// where the cluster ends, which is where the header's first control may begin, and
// the marks' centre line, which is what that control centres on.
//
// Deliberately NOT the titlebar height, which would be the app's header height by
// another name — that number belongs to the visual style, and handing the frame a
// second claim on it would leave one value with two owners.
func nativeWindowChrome() (controlsCentreY, controlsInlineEnd float64, measured bool) {
	metrics := C.windowChromeMetrics()
	return float64(metrics.controlsCentreY), float64(metrics.controlsInlineEnd), metrics.measured != 0
}

// useCompactWindowToolbar makes the titlebar 40pt rather than the 32pt a
// toolbarless window gets, which puts the frame buttons 20pt down — within a pixel
// of the centre line of a 42pt header. That last pixel is why the marks' centre is
// measured rather than assumed: the control beside them centres on THEM, and the
// header's text on the header, and at 5pt apart (which is where a toolbarless
// window leaves them) no amount of measuring makes those two read as one row.
//
// An empty toolbar was verified not to take the clicks in that band: hit-testing
// the frame view 8, 16, 24, 36 and 44pt below the window top returns the content
// view in all three toolbar styles, so the header's own controls keep working
// under it.
//
// Wails owns whether there is a toolbar at all (`mac.TitleBar.UseToolbar`) but not
// its style, and the automatic style resolves to a titlebar two thirds taller.
func useCompactWindowToolbar() {
	C.applyCompactToolbar()
}
