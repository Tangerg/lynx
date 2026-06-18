// Built-in slash hints. These are *display-only* — typing one of them shows
// the description in the autocomplete dropdown, but pressing Enter just
// sends the text as a normal user message. Concrete commands with `run`
// handlers come from plugins.

import { definePlugin } from "@/plugins/sdk";
import { t } from "@/lib/i18n";
import { SLASH_COMMAND } from "@/plugins/sdk/kernelPoints";

const HINTS: Array<[cmd: string, description: string]> = [
  ["/explain", t("slash.explain")],
  ["/test", t("slash.test")],
  ["/fix", t("slash.fix")],
  ["/diff", t("slash.diff")],
  ["/review", t("slash.review")],
  ["/commit", t("slash.commit")],
  ["/search", t("slash.search")],
  ["/plan", t("slash.plan")],
];

export default definePlugin({
  name: "lyra.builtin.slash-hints",
  version: "1.0.0",
  setup({ host }) {
    for (const [cmd, description] of HINTS) {
      host.extensions.contribute(SLASH_COMMAND, { description }, { key: cmd });
    }
  },
});
