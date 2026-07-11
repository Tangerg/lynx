// Hello-sideload — minimal sideloaded plugin proving the end-to-end load
// path works. Drop the parent folder into ~/.lyra/plugins/ to install:
//
//   cp -r frontend/sample-plugins/hello-sideload ~/.lyra/plugins/
//   # restart wails dev
//
// What it does:
//   - Registers /hello slash command (run handler shows a toast)
//   - Registers a tool preview for fn === "hello" (just for shape; no agent
//     today will emit a "hello" tool call, but the registration proves the
//     surface)
//
// Plain ES module — no JSX, no imports. Pulls React + SDK off window.__LYRA__.
//
// To use JSX or TypeScript, pre-bundle this folder yourself with esbuild
// and ship the resulting `index.js` instead.

const { React, SDK } = window.__LYRA__;
const { definePlugin, SLASH_COMMAND, TOOL_PREVIEW } = SDK;

// React.createElement shorthand keeps the code readable without JSX.
const h = React.createElement;

function HelloPreview(props) {
  return h(
    "div",
    {
      style: {
        padding: "10px 12px",
        borderRadius: 8,
        background: "rgba(82, 157, 245, 0.10)",
        border: "1px solid rgba(82, 157, 245, 0.36)",
        color: "var(--color-info)",
        fontSize: 13,
      },
    },
    `Hello from sideload! args: ${props.tool.args || "(none)"}`,
  );
}

export default definePlugin({
  name: "user.hello-sideload",
  version: "0.1.0",
  apiVersion: "^3.0.0",
  capabilities: ["extensions", "tool", "composer", "notify"],
  setup({ host }) {
    host.extensions.contribute(TOOL_PREVIEW, HelloPreview, { key: "hello" });
    host.extensions.contribute(
      SLASH_COMMAND,
      {
        description: "Sideload demo — shows a toast",
        run: async ({ args }) => {
          host.notify(`Hello${args ? `, ${args}` : ""}!`, "info");
        },
      },
      { key: "/hello" },
    );
  },
});
