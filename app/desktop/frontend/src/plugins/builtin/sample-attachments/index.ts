// Built-in plugin: the two sample chips ("src/api/auth.ts", "TYPES.md")
// that previously seeded `useComposerStore.attachments`. They're now an
// attachment *source* — the chips show up in the composer until a future
// "real attachments" plugin replaces them.
//
// Disabling this plugin (just removing it from the manifest) is enough to
// hide the samples — useful for demos / screenshots.

import type { ComposerAttachment } from "@/plugins/sdk";
import { definePlugin } from "@/plugins/sdk";
import { COMPOSER_ATTACHMENT_SOURCE } from "@/plugins/sdk/kernelPoints";

const SAMPLES: ComposerAttachment[] = [
  { label: "src/api/auth.ts", icon: "file" },
  { label: "TYPES.md", icon: "filetext" },
];

export default definePlugin({
  name: "lyra.builtin.sample-attachments",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(COMPOSER_ATTACHMENT_SOURCE, {
      id: "samples",
      order: 0,
      useAttachments: () => SAMPLES,
    });
  },
});
