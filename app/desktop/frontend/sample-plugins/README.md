# Sample sideload plugins

Drop any folder here into `~/.lyra/plugins/` and restart the app to load it.

## Installing

```bash
cp -r frontend/sample-plugins/hello-sideload ~/.lyra/plugins/
# restart wails dev
```

After restart, open **Settings → Plugins** — `user.hello-sideload` should
appear with a blue "Sideload" badge. Type `/hello world` in the composer to
trigger the toast.

## Writing your own

A sideloaded plugin is a folder containing an `index.js` (ES module) that
default-exports the result of `definePlugin(...)`.

The browser dynamic-imports your file from
`http://127.0.0.1:17171/plugins/<folder>/index.js`, so:

- You can't `import` from npm. The host's React, motion, SDK, etc. are
  exposed on `window.__LYRA__`.
- If you want JSX or TypeScript, **pre-bundle** with esbuild and ship the
  resulting `.js`:

  ```bash
  esbuild src/index.tsx --bundle --format=esm --outfile=index.js \
    --external:window
  ```

- Declare `apiVersion: "^1.0.0"` in your `definePlugin` call — the host
  refuses to load incompatible ranges (see `docs/PLUGINS_IMPL.md`).

## Available on `window.__LYRA__`

| Field | What it is |
|---|---|
| `React` | `import * as React from "react"` |
| `ReactJSXRuntime` | for pre-compiled JSX via the automatic runtime |
| `Motion` | `import * as Motion from "motion/react"` |
| `SDK` | `import * as SDK from "@lyra/plugin-sdk"` (everything you'd import from the SDK package) |
| `apiVersion` | The host's API version string |

## Plugin shape recap

```js
const { React, SDK } = window.__LYRA__;
const { definePlugin } = SDK;

export default definePlugin({
  name:       "user.something",           // unique id
  version:    "0.1.0",                    // semver
  apiVersion: "^1.0.0",                   // host range (optional but recommended)
  setup({ host }) {
    host.tool.registerPreview("my-tool", MyToolPreview);
    host.composer.registerCommand("/cmd", { description: "...", run: ... });
    host.message.registerContentBlock("myBlock", MyBlockRenderer);
    host.agui.on("my.event", (value) => /* StateUpdate */);
    host.settings.registerPane({ id: "my-pane", label: "...", component: ... });
  },
});
```

See `docs/EXTENSION_POINTS.md` (substrate + landed status) and `docs/PLUGINS_IMPL.md`
(implementation guide + SDK reference) for the full surface and design rationale.
