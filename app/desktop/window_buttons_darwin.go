package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

static void hideFrameButtons(NSWindow *window) {
	[[window standardWindowButton:NSWindowCloseButton] setHidden:YES];
	[[window standardWindowButton:NSWindowMiniaturizeButton] setHidden:YES];
	[[window standardWindowButton:NSWindowZoomButton] setHidden:YES];
}

static void hideStandardWindowButtons(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		// Every window the application owns at this instant, which is the one
		// Wails created before it started the run loop. Panels and sheets are
		// opened later and keep their own controls.
		for (NSWindow *window in [NSApp windows]) {
			hideFrameButtons(window);
			// AppKit rebuilds the titlebar on the way into and out of
			// fullscreen and hands the rebuilt one the frame's own buttons, so
			// hiding once at startup only holds until the first ^⌘F.
			for (NSNotificationName transition in @[NSWindowDidEnterFullScreenNotification,
			                                        NSWindowDidExitFullScreenNotification]) {
				[[NSNotificationCenter defaultCenter] addObserverForName:transition
				                                                  object:window
				                                                   queue:[NSOperationQueue mainQueue]
				                                              usingBlock:^(NSNotification *note) {
					hideFrameButtons(note.object);
				}];
			}
		}
	});
}
*/
import "C"

// hideNativeWindowButtons takes the platform's three controls off the window
// frame, leaving the frame itself intact.
//
// The app draws its own controls, but only the buttons were ever ours to
// replace: dropping NSWindowStyleMaskTitled to be rid of them takes the whole
// frame with it, and with the frame go the rounded corners, the drop shadow and
// the corner resize regions. Hiding the buttons on a real titled window keeps
// all of that, at whatever corner radius the running macOS draws — a number no
// stylesheet of ours could track.
//
// Safe to call from any goroutine and before the window exists; the work is
// queued onto the main thread, which is the only one allowed to touch AppKit.
func hideNativeWindowButtons() {
	C.hideStandardWindowButtons()
}
