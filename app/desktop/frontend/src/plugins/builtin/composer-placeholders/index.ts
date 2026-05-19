// Built-in plugin: a small pool of textarea placeholders for the composer.
// Composer picks one at random on mount; weight defaults to 1 each.
//
// The "default" entry preserves the original (single) placeholder text;
// the others are nudges toward specific commands. Plugins can add their
// own — e.g. a marketing-mode plugin that biases toward "Try /demo".

import { definePlugin } from "@/plugins/sdk";

export default definePlugin({
  name: "lyra.builtin.composer-placeholders",
  version: "1.0.0",
  setup({ host }) {
    host.composer.registerPlaceholder({
      id: "default",
      text: "Ask, plan, or paste a stack trace…  /  to run a command",
      weight: 4,
    });
    host.composer.registerPlaceholder({
      id: "plan",
      text: "What should I do next? Try /plan to draft a checklist.",
    });
    host.composer.registerPlaceholder({
      id: "search",
      text: "Look up something in the codebase — start with /search …",
    });
    host.composer.registerPlaceholder({
      id: "explain",
      text: "Paste an error or a snippet and I'll explain it.",
    });
  },
});
