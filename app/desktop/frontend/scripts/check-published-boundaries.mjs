#!/usr/bin/env node
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";

const SRC = join(process.cwd(), "src");
const TEXT_EXT = /\.(ts|tsx|md)$/;

function files(dir) {
  const out = [];
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry);
    const stat = statSync(path);
    if (stat.isDirectory()) out.push(...files(path));
    else if (TEXT_EXT.test(path)) out.push(path);
  }
  return out;
}

const violations = [];

for (const file of files(SRC)) {
  const rel = relative(SRC, file);
  const text = readFileSync(file, "utf8");
  const isTest = /\.(test|spec)\.[tj]sx?$/.test(rel);

  if (/@\/lib\/data\/(?:queries|useUsage)/.test(text)) {
    violations.push({
      file: rel,
      reason: "legacy global business-query modules must not be referenced",
    });
  }

  if (!isTest && rel.startsWith("plugins/builtin/") && /\blet\s+port\s*:/.test(text)) {
    violations.push({
      file: rel,
      reason: "builtin application ports must use replacement-safe singletonPort lifecycle",
    });
  }

  if (
    !isTest &&
    /plugins\/builtin\/.+\/application\/ports\/.+\.ts$/.test(rel) &&
    /configure\w+(?:Port|Gateway)/.test(text) &&
    !/@\/lib\/ports\/singletonPort/.test(text)
  ) {
    violations.push({
      file: rel,
      reason: "configurable application ports must use singletonPort lifecycle ownership",
    });
  }

  if (
    !isTest &&
    /plugins\/builtin\/.+\/adapters\/.+\.(ts|tsx)$/.test(rel) &&
    /export\s+function\s+install\w+\(\)\s*:\s*void/.test(text)
  ) {
    violations.push({
      file: rel,
      reason: "port installers must return a disposer for plugin unload and HMR",
    });
  }

  if (
    !isTest &&
    /plugins\/builtin\/.+\/(?:index|bootstrap)\.(ts|tsx)$/.test(rel) &&
    /^\s*install\w+(?:Port|Gateway)\(\);/m.test(text)
  ) {
    violations.push({
      file: rel,
      reason: "plugin setup must retain and return the port installer's disposer",
    });
  }

  // An id is a context's vocabulary, and a literal at a foreign callsite is a
  // dependency on it that nothing checks: rename the view or the pane and the call
  // still compiles, the click just lands nowhere. Seven had drifted in — three view
  // ids and four pane ids. The owners publish the words: named openers in the
  // workspace's `public/deeplinks`, id constants in the settings pack's
  // `public/panes` (the same shape as a data-provider key).
  if (
    !isTest &&
    !rel.startsWith("plugins/builtin/workspace/") &&
    /\bopenWorkspaceView(?:Beside)?\(\s*["'`]/.test(text)
  ) {
    violations.push({
      file: rel,
      reason: "workspace view id spelled at a foreign callsite; use a public/deeplinks opener",
    });
  }

  if (!isTest && /\bopenWorkspaceSettingsPane\(\s*["'`]/.test(text)) {
    violations.push({
      file: rel,
      reason: "settings pane id spelled as a literal; use the constant from settings/public/panes",
    });
  }

  if (/@\/protocol\/run|protocol\/run|agent\/core-reducer|core-reducer/.test(text)) {
    violations.push({
      file: rel,
      reason: "old agent fold/view-state path or name is referenced",
    });
  }

  if (/plugins\/builtin\/chat\/composer\/public\/input\.ts$/.test(rel) && /@\/rpc/.test(text)) {
    violations.push({
      file: rel,
      reason: "composer public input must expose composer language, not runtime wire types",
    });
  }

  if (
    /plugins\/builtin\/.+\/public\/.+\.(ts|tsx)$/.test(rel) &&
    /from\s+["']@\/rpc["']/.test(text)
  ) {
    violations.push({
      file: rel,
      reason: "builtin public surfaces must not import runtime wire directly",
    });
  }

  if (
    /plugins\/builtin\/.+\/application\/.+(?:Queries|Data)\.ts$/.test(rel) &&
    /from\s+["']@\/rpc["']/.test(text)
  ) {
    violations.push({
      file: rel,
      reason: "context read models must publish context language, not runtime wire DTOs",
    });
  }

  if (
    !rel.startsWith("plugins/sdk/") &&
    !rel.startsWith("plugins/host/") &&
    !rel.startsWith("plugins/builtin/agent/") &&
    rel !== "plugins/builtin/agent/public/viewState.ts" &&
    /from\s+["']@\/plugins\/sdk\/types\/agent(View|Timeline)["']/.test(text)
  ) {
    violations.push({
      file: rel,
      reason: "cross-context consumers must use the agent public view-state facade",
    });
  }

  if (
    !rel.startsWith("plugins/builtin/agent/public/") &&
    /plugins\/builtin\/agent\/.+\.(ts|tsx)$/.test(rel) &&
    /from\s+["']@\/plugins\/builtin\/agent\/public\/viewState["']/.test(text)
  ) {
    violations.push({
      file: rel,
      reason: "agent internals must depend on the SDK type owner, not their outward public facade",
    });
  }

  if (
    !isTest &&
    /plugins\/builtin\/.+\/domain\/.+\.(ts|tsx)$/.test(rel) &&
    /from\s+["'](?:react|zustand(?:\/[^"']*)?|@\/(?:rpc|state|main|ui|components|pages)(?:\/[^"']*)?)["']/.test(
      text,
    )
  ) {
    violations.push({
      file: rel,
      reason: "builtin context domain must stay framework-, store-, wire-, and UI-free",
    });
  }

  // A pure projection is a function of its inputs. The wall clock is neither an
  // input nor deterministic: a fold that stamps `new Date()` cannot be replayed,
  // its tests have to strip the field, and — the reason this got noticed — a
  // synthesized assistant turn ended up dated by the CLIENT clock while every
  // message beside it carried the runtime's, so the date separator above it could
  // disagree with the messages under it on a skewed machine. A synthesized entity
  // takes the timestamp of the wire event that caused it, or carries none.
  //
  // Actions are exempt by not being here: "when did this export happen" is the
  // event's own timestamp, and `runSummaryViewModel` shows the other way out —
  // take `now` as a parameter.
  if (
    !isTest &&
    /plugins\/builtin\/.+\/(?:domain|presentation|application\/fold)\/.+\.(ts|tsx)$/.test(rel) &&
    /new Date\(\)|Date\.now\(\)|Math\.random\(\)/.test(
      text.replace(/\/\/[^\n]*/g, "").replace(/\/\*[\s\S]*?\*\//g, ""),
    )
  ) {
    violations.push({
      file: rel,
      reason: "a pure projection must not read the clock or randomness",
    });
  }

  // A use case is answerable without a browser. `document`, `window`, `Blob` and
  // friends are the browser's mechanisms, and a layer that reaches for them cannot
  // be exercised — or reasoned about — without one. Two modules had drifted here:
  // the conversation export owned "this is how Chromium saves a file", and the
  // font picker owned `document.fonts.check()`. Both are ports now, bound by an
  // adapter. Comments and type positions are exempt; this looks for real access.
  if (
    !isTest &&
    /plugins\/builtin\/.+\/(?:application|domain)\/.+\.(ts|tsx)$/.test(rel) &&
    /(?:^|[^\w.'"`/])(?:document|window)\s*[.?[]|new (?:Blob|FileReader|Image)\(|URL\.(?:create|revoke)ObjectURL/m.test(
      text.replace(/\/\/[^\n]*/g, "").replace(/\/\*[\s\S]*?\*\//g, ""),
    )
  ) {
    violations.push({
      file: rel,
      reason: "builtin context application / domain must reach the browser through a port",
    });
  }

  // `presentation/` maps a model into a view model. It does not render one, and it
  // does not decide what anything looks like — a component or a class string there
  // is a component or a class string that four callers cannot share without
  // importing this context. `agent/presentation/planPresentation.tsx` was exactly
  // that: a React component plus a class-string builder, reached by three other
  // contexts through the agent's facade.
  if (
    !isTest &&
    /plugins\/builtin\/.+\/(?:application|presentation)\/.+\.(ts|tsx)$/.test(rel) &&
    /from\s+["']@\/(?:ui|components|pages)(?:\/[^"']*)?["']/.test(text)
  ) {
    violations.push({
      file: rel,
      reason: "builtin context application / presentation must not import UI components or pages",
    });
  }

  if (
    !isTest &&
    /plugins\/builtin\/.+\/application\/.+\.(ts|tsx)$/.test(rel) &&
    /from\s+["'](?:@\/plugins\/builtin\/.+\/adapters(?:\/[^"']*)?|(?:\.\.\/)+adapters(?:\/[^"']*)?)["']/.test(
      text,
    )
  ) {
    violations.push({
      file: rel,
      reason: "builtin context application must depend on ports, not adapter implementations",
    });
  }

  if (
    !isTest &&
    /plugins\/builtin\/.+\/application\/.+\.(ts|tsx)$/.test(rel) &&
    /from\s+["']@\/state(?:\/[^"']*)?["']/.test(text)
  ) {
    violations.push({
      file: rel,
      reason: "builtin context application must depend on context ports, not global stores",
    });
  }

  if (
    !isTest &&
    /plugins\/builtin\/agent\/application\/.+\.(ts|tsx)$/.test(rel) &&
    /from\s+["']@\/main\/container["']/.test(text)
  ) {
    violations.push({
      file: rel,
      reason: "agent application must depend on runtime gateway ports, not the composition root",
    });
  }

  if (
    !isTest &&
    /plugins\/builtin\/settings\/providers\/application\/.+\.(ts|tsx)$/.test(rel) &&
    /from\s+["']@\/main\/container["']/.test(text)
  ) {
    violations.push({
      file: rel,
      reason:
        "provider settings application must depend on provider gateway ports, not the composition root",
    });
  }

  if (
    !isTest &&
    /plugins\/builtin\/settings\/mcp-servers\/application\/.+\.(ts|tsx)$/.test(rel) &&
    /from\s+["']@\/main\/container["']/.test(text)
  ) {
    violations.push({
      file: rel,
      reason:
        "MCP server settings application must depend on MCP gateway ports, not the composition root",
    });
  }

  if (
    !isTest &&
    /plugins\/builtin\/settings\/(?:schedules|hooks)\/application\/.+\.(ts|tsx)$/.test(rel) &&
    /from\s+["']@\/main\/container["']/.test(text)
  ) {
    violations.push({
      file: rel,
      reason: "settings application must depend on context gateway ports, not the composition root",
    });
  }

  if (
    !isTest &&
    /plugins\/builtin\/workspace\/application\/.+\.(ts|tsx)$/.test(rel) &&
    /from\s+["']@\/main\/container["']/.test(text)
  ) {
    violations.push({
      file: rel,
      reason: "workspace application must depend on context gateway ports",
    });
  }

  if (
    !isTest &&
    /plugins\/builtin\/workspace\/application\/.+\.(ts|tsx)$/.test(rel) &&
    /from\s+["']@\/rpc["']/.test(text)
  ) {
    violations.push({
      file: rel,
      reason: "workspace application must expose workspace language, not runtime wire types",
    });
  }

  if (
    !isTest &&
    /plugins\/builtin\/(?:chat\/recipes|settings\/usage)\/application\/.+\.(ts|tsx)$/.test(rel) &&
    /from\s+["']@\/rpc["']/.test(text)
  ) {
    violations.push({
      file: rel,
      reason: "context application must expose context language, not runtime wire types",
    });
  }

  if (
    !isTest &&
    /plugins\/builtin\/defaults\/(?!adapters\/).+\.(ts|tsx)$/.test(rel) &&
    /from\s+["']@\/main\/container["']/.test(text)
  ) {
    violations.push({
      file: rel,
      reason: "defaults root must stay an assembly entry point; runtime wiring belongs in adapters",
    });
  }

  if (
    !isTest &&
    /plugins\/builtin\/chat\/composer\/.+\.(ts|tsx)$/.test(rel) &&
    /from\s+["']@\/plugins\/builtin\/chat\/message-actions(?:\/[^"']*)?["']/.test(text)
  ) {
    violations.push({
      file: rel,
      reason: "composer must not depend on chat message-actions orchestration",
    });
  }

  if (
    !isTest &&
    /plugins\/builtin\/.+\/public\/.+\.(ts|tsx)$/.test(rel) &&
    /from\s+["'](?:@\/plugins\/builtin\/.+\/adapters(?:\/[^"']*)?|(?:\.\.\/)+adapters(?:\/[^"']*)?)["']/.test(
      text,
    )
  ) {
    violations.push({
      file: rel,
      reason: "builtin public surfaces must expose published ports, not adapter implementations",
    });
  }
}

if (violations.length > 0) {
  console.error(`[check-published-boundaries] Found ${violations.length} violation(s):`);
  for (const violation of violations) console.error(`  ${violation.file}: ${violation.reason}`);
  process.exit(1);
}

console.log("[check-published-boundaries] OK — published boundaries stay wire-free.");
