// Built-in plugin: the default composer keymap.
//
// Just one binding for now — Enter (without shift) submits — but the
// shape generalises: a vim-keys plugin could register `Mod+J` / `Mod+K`
// for line navigation, a productivity plugin could bind `Mod+Backspace`
// to clear the textarea, etc.

import { definePlugin } from "@/plugins/sdk";

export default definePlugin({
  name: "lyra.builtin.composer-keymap",
  version: "1.0.0",
  setup({ host }) {
    host.composer.registerKeyBinding({
      key: "Enter",
      description: "Send the current message",
      handler: ({ submit }) => {
        submit();
        return true;
      },
    });
  },
});
