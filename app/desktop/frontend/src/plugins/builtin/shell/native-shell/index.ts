// Built-in plugin: native-shell — the handful of behaviours that make a webview
// behave like a window rather than a page.
//
// 1. Suppress the WebView's default right-click menu. A browser context menu
//    popping up over app chrome (tabs, sidebar, messages) is one of the loudest
//    "there's a webview under this window" tells. A desktop surface either shows
//    an app-defined menu or nothing.
//
//    Exception: real text fields keep the system edit menu (cut / copy / paste /
//    look-up) — on WKWebView that IS the native macOS edit menu, the affordance
//    users expect on an input, not a web tell.
//
//    Base UI context menus are unaffected: their trigger handles the event and
//    opens their own menu (React listeners on the root container run before this
//    document-level bubble listener), so calling preventDefault here is a no-op
//    for them — it only kills the *default* browser menu where nothing is wired.
//
// 2. Publish when the window is NOT the focused one, which chrome that mimics
//    platform controls has to answer to.
//

import { definePlugin } from "@/plugins/sdk";

const EDITABLE = "input, textarea, [contenteditable='true']";

/** Mirrors window focus onto the document so CSS can reach it. Read from
 *  `document.hasFocus()` rather than tracked from events alone, so a window that
 *  opens behind another app is not painted as the focused one.
 *
 *  The attribute marks the INACTIVE state, not the active one: absence has to
 *  mean "coloured", or a platform that never reports focus — or this plugin
 *  failing to load — would ship a window whose controls are permanently grey. */
const WINDOW_INACTIVE_ATTR = "data-window-inactive";

export default definePlugin({
  name: "lyra.builtin.native-shell",
  setup(ctx) {
    const onContextMenu = (e: MouseEvent) => {
      const target = e.target as HTMLElement | null;
      if (target?.closest(EDITABLE)) return; // keep the system edit menu on inputs
      e.preventDefault();
    };
    document.addEventListener("contextmenu", onContextMenu);

    const syncFocus = () => {
      document.documentElement.toggleAttribute(WINDOW_INACTIVE_ATTR, !document.hasFocus());
    };
    syncFocus();
    addEventListener("focus", syncFocus);
    addEventListener("blur", syncFocus);

    ctx.cleanup(() => {
      document.removeEventListener("contextmenu", onContextMenu);
      removeEventListener("focus", syncFocus);
      removeEventListener("blur", syncFocus);
      document.documentElement.removeAttribute(WINDOW_INACTIVE_ATTR);
    });
  },
});
