// Built-in plugin: native-shell — suppress the WebView's default right-click
// menu. A browser context menu popping up over app chrome (tabs, sidebar,
// messages) is one of the loudest "there's a webview under this window" tells
// (NATIVE_FEEL.md §A). A desktop surface either shows an app-defined menu or
// nothing.
//
// Exception: real text fields keep the system edit menu (cut / copy / paste /
// look-up) — on WKWebView that IS the native macOS edit menu, the affordance
// users expect on an input, not a web tell.
//
// Base UI context menus are unaffected: their trigger handles the event and
// opens their own menu (React listeners on the root container run before this
// document-level bubble listener), so calling preventDefault here is a no-op
// for them — it only kills the *default* browser menu where nothing is wired.

import { definePlugin } from "@/plugins/sdk";

const EDITABLE = "input, textarea, [contenteditable='true']";

export default definePlugin({
  name: "lyra.builtin.native-shell",
  version: "1.0.0",
  setup() {
    const onContextMenu = (e: MouseEvent) => {
      const target = e.target as HTMLElement | null;
      if (target?.closest(EDITABLE)) return; // keep the system edit menu on inputs
      e.preventDefault();
    };
    document.addEventListener("contextmenu", onContextMenu);
    return () => document.removeEventListener("contextmenu", onContextMenu);
  },
});
