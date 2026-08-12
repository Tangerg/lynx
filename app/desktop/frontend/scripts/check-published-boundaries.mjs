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

// One vocabulary, one name.
//
// A set of string literals that means something to this app — light/dark, the
// approval stances, the notification levels — is a type, and a second spelling of
// it is a second thing that must agree with the first forever, with nothing
// checking that it does. `"dark" | "light"` had been written out eight times (the
// SDK's theme contract, the theme kit, a port, two `Record` keys, a local
// annotation) and the approval stances four; the workspace's doc scope had four
// spellings, one of them an indexed-access alias of an inline union.
//
// Flagged when the two are one context's own business — same name, same bounded
// context, or one of them in `lib/` (shared by every ring, so nothing may
// re-spell it). A pair that straddles a real boundary is a translation and stays:
// `src/rpc/` is exempt on both sides, because a context read model is REQUIRED to
// publish its own language, not the wire's (see the Queries/Data rule below), and
// a consumer-defined port names what it needs.
const namedUnions = [];
const inlineUnions = [];
const UNION_DECL = /(?:export )?type (\w+) =\s*((?:\s*\|?\s*"[\w-]+")+)\s*;/g;
const UNION_INLINE = /(?:"[\w-]+"\s*\|\s*)+"[\w-]+"/g;

function memberKey(text) {
  return [...text.matchAll(/"([^"]+)"/g)].map((m) => m[1]).sort();
}

function collectVocabulary(rel, source) {
  // Indices must line up with what the patterns matched, so strip comments first
  // and match the stripped text throughout.
  const text = source.replace(/\/\/[^\n]*/g, "").replace(/\/\*[\s\S]*?\*\//g, "");
  const declared = [];
  for (const m of text.matchAll(UNION_DECL)) {
    const members = memberKey(m[2]);
    declared.push([m.index, m.index + m[0].length]);
    if (members.length > 1) namedUnions.push({ name: m[1], key: members.join("|"), rel });
  }
  for (const m of text.matchAll(UNION_INLINE)) {
    // A multi-line declaration matches its own right-hand side.
    if (declared.some(([from, to]) => m.index >= from && m.index < to)) continue;
    const members = memberKey(m[0]);
    if (members.length > 1) inlineUnions.push({ key: members.join("|"), rel });
  }
}

function boundedContext(rel) {
  const builtin = /^plugins\/builtin\/([^/]+)\//.exec(rel);
  if (builtin) return `builtin:${builtin[1]}`;
  if (rel.startsWith("plugins/")) return "plugin-platform";
  return rel.split("/")[0];
}

/** Source with comments removed — a rule about calls must not read prose. */
function code(text) {
  return text.replace(/\/\*[\s\S]*?\*\//g, "").replace(/\/\/[^\n]*/g, "");
}

const isWire = (rel) => rel.startsWith("rpc/");
const isShared = (rel) => rel.startsWith("lib/");

function reportDuplicateVocabulary() {
  const byKey = new Map();
  for (const decl of namedUnions) {
    if (!byKey.has(decl.key)) byKey.set(decl.key, []);
    byKey.get(decl.key).push(decl);
  }
  const label = (key) => (key.length > 60 ? `${key.slice(0, 57)}…` : key);

  for (const group of byKey.values()) {
    for (let i = 0; i < group.length; i++) {
      for (let j = i + 1; j < group.length; j++) {
        const [a, b] = [group[i], group[j]];
        if (isWire(a.rel) || isWire(b.rel)) continue;
        const oneOwner =
          a.name === b.name ||
          boundedContext(a.rel) === boundedContext(b.rel) ||
          isShared(a.rel) ||
          isShared(b.rel);
        if (!oneOwner) continue;
        violations.push({
          file: a.rel,
          reason: `${a.name} here and ${b.name} in ${b.rel} are two names for ${label(a.key)} — one vocabulary, one name`,
        });
      }
    }
  }

  for (const use of inlineUnions) {
    if (isWire(use.rel)) continue;
    for (const owner of byKey.get(use.key) ?? []) {
      if (isWire(owner.rel)) continue;
      if (boundedContext(owner.rel) !== boundedContext(use.rel) && !isShared(owner.rel)) continue;
      violations.push({
        file: use.rel,
        reason: `inline ${label(use.key)} restates ${owner.name} from ${owner.rel} — name the vocabulary, don't respell it`,
      });
    }
  }
}

for (const file of files(SRC)) {
  const rel = relative(SRC, file);
  const text = readFileSync(file, "utf8");
  const isTest = /\.(test|spec)\.[tj]sx?$/.test(rel);

  if (!isTest && /\.tsx?$/.test(rel)) {
    collectVocabulary(rel, text);

    // Runtime wire vocabulary is translated at the Agent Adapter boundary.
    // Letting Application, Domain, or Public import `rpc` makes transport DTOs
    // the product model by accident; publishing them from the SDK stream seam
    // leaks the same dependency to every plugin handler. These were once spread
    // across live events, durable snapshots, cancel responses, and HITL reads,
    // so guard every inward/public ring and both neutral SDK event contracts.
    const importsRuntimeWire = /from\s+["']@\/rpc(?:\/[^"']*)?["']/.test(code(text));
    if (
      importsRuntimeWire &&
      /^plugins\/builtin\/agent\/(?:application|domain|public)\//.test(rel)
    ) {
      violations.push({
        file: rel,
        reason:
          "Agent Application/Domain/Public imports Runtime wire vocabulary — translate it in agent/adapters",
      });
    }
    if (
      importsRuntimeWire &&
      (rel === "plugins/sdk/types/events.ts" || rel === "plugins/sdk/types/agentEvents.ts")
    ) {
      violations.push({
        file: rel,
        reason:
          "the SDK Agent event surface publishes Runtime wire vocabulary — publish neutral Agent facts instead",
      });
    }

    if (/(?:^|\/)_?(?:utils?|helpers?|shared|impl|data|info)\.(?:ts|tsx)$/.test(rel)) {
      violations.push({
        file: rel,
        reason:
          "generic module name hides its responsibility — name the owned policy, model, adapter, or UI element",
      });
    }

    if (
      /plugins\/builtin\/.+\/(?:application|domain)\/.+\.(?:ts|tsx)$/.test(rel) &&
      /\b(?:interface|type|class)\s+\w+(?:Manager|Helper|Impl|Data|Info)\b/.test(code(text))
    ) {
      violations.push({
        file: rel,
        reason:
          "application/domain type uses a role-less suffix — name the domain fact or use-case responsibility",
      });
    }

    // A type has one identity; a second name for it is a hop that hides where the
    // concept lives. Seven modules published another module's type under a locally
    // preferred noun (`ApprovalMode = ApprovalModeValue`, `HookConfig =
    // HookReadModel`, and one that renamed on import only to rename back), so the
    // same shape read as two things depending on which file you were standing in —
    // and two contexts had independently minted `MCPServerConfig` for two
    // different types. Re-exporting under the SAME name is fine: no new word.
    const imported = new Set();
    for (const stmt of text.matchAll(/import[^;]*?from\s+["'][^"']+["']/g)) {
      for (const id of stmt[0].matchAll(/\b(\w+)\b(?=\s*(?:,|\}|\s+as\b))/g)) imported.add(id[1]);
    }
    for (const alias of text.matchAll(/^export type (\w+) = (\w+);$/gm)) {
      if (imported.has(alias[2])) {
        violations.push({
          file: rel,
          reason: `${alias[1]} is a second name for the imported ${alias[2]} — import the owner's name, or re-export it unrenamed`,
        });
      }
    }
  }

  // A Page is a page. Registry-classified paged methods return an AutoPagingPromise:
  // awaiting it deliberately reads the first wire page, while normal whole-list
  // consumers use autoPagingToArray / autoPagingEach / async iteration. Reading
  // `data` without an explicit limit silently drops nextCursor and turns an
  // incomplete result into an apparently complete one.
  //
  // Two shapes: reading `.data` straight off one of the historically unbounded
  // calls, and a module that calls one while mentioning no SDK paging operation.
  // The latter remains file-granular because a regex cannot associate a later
  // cursor fold with one expression. A bounded `limit` read stays valid when the
  // caller is explicitly asking an existence/preview question.
  if (
    !isTest &&
    !rel.startsWith("rpc/") &&
    // A call that names its own `limit` is reading a bounded slice deliberately —
    // asking for one row to answer "is there anything here?" is not the mistake
    // this looks for.
    (/\b(?:sessions\.list|items\.list|runs\.list|listOpenInterrupts|schedules\.list)\((?![^)]*\blimit\b)[^)]*\)\s*\)?\s*\.data\b/.test(
      code(text),
    ) ||
      (/\b(?:sessions\.list|items\.list|runs\.list|listOpenInterrupts|schedules\.list)\(/.test(
        code(text),
      ) &&
        !/\b(?:autoPagingToArray|autoPagingEach|nextCursor)\b/.test(code(text))))
  ) {
    violations.push({
      file: rel,
      reason:
        "reads a paged method without using its auto-paging SDK surface — drain it or request an explicit bounded limit",
    });
  }

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
    /plugins\/(?:builtin\/.+\/adapters|host)\/.+\.(ts|tsx)$/.test(rel) &&
    // `install*` names one thing in this tree: it installs a port and hands back the
    // disposer. The parameterless form was the only one checked, so an installer that
    // took the Host slipped through — and it wasn't installing a port at all, it was
    // subscribing through a facade the Host already disposes. Two of those had drifted
    // to the prefix (`installHostBridge`, `installEndpointMirror`); both are named for
    // what they do now, which is what makes this a reliable signal to read.
    /export\s+function\s+install\w+\([^)]*\)\s*:\s*void/.test(text)
  ) {
    violations.push({
      file: rel,
      reason:
        "an install* function installs a port and returns its disposer — rename it, or return one",
    });
  }

  if (
    !isTest &&
    /plugins\/builtin\/.+\/(?:index|bootstrap)\.(ts|tsx)$/.test(rel) &&
    // Any argument list, any suffix: dropping the disposer leaks the same way whether
    // the installer is called `installFooPort()` or `installFoo(host)`.
    /^\s*install\w+\([^)]*\);/m.test(text)
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
    /from\s+["']@\/rpc(?:\/[^"']*)?["']/.test(text)
  ) {
    violations.push({
      file: rel,
      reason: "builtin public surfaces must not import runtime wire directly",
    });
  }

  if (
    /plugins\/builtin\/.+\/application\/.+(?:Queries|Data)\.ts$/.test(rel) &&
    /from\s+["']@\/rpc(?:\/[^"']*)?["']/.test(text)
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

  // `main/` is the composition root. A bounded context's application and
  // domain rings describe policy and use cases that must be executable without
  // the assembled app; concrete access belongs in that context's adapters.
  // This is intentionally one rule for every context — the previous
  // context-by-context list only guarded `main/container`, so Runtime
  // application code reached `main/config` and the hole looked legitimate.
  if (
    !isTest &&
    /plugins\/builtin\/.+\/(?:application|domain)\/.+\.(ts|tsx)$/.test(rel) &&
    /from\s+["']@\/main(?:\/[^"']*)?["']/.test(text)
  ) {
    violations.push({
      file: rel,
      reason: "builtin context application / domain must reach the composition root through a port",
    });
  }

  // Protocol vocabulary can legitimately be an input to an anti-corruption
  // fold. A raw client or response envelope cannot: method dispatch and
  // envelope removal are adapter responsibilities.
  if (
    !isTest &&
    /plugins\/builtin\/.+\/application\/.+\.(ts|tsx)$/.test(rel) &&
    /from\s+["']@\/rpc["']/.test(text) &&
    /\b(?:RpcClient|DiscoverResponse)\b/.test(code(text))
  ) {
    violations.push({
      file: rel,
      reason:
        "builtin context application must depend on a gateway, not raw RPC clients or envelopes",
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

  // Reaching the composition root is an adapter's job — every context keeps that
  // pair (get the client, coerce to the wire's branded ids) in `adapters/`. This was
  // written for `defaults/` only, so the same shape read as a local exception
  // elsewhere: the rpc-agent's root held a runs gateway as an object literal inside
  // `setup()`, and the runtime's root reached for `client().rpc` inline. A root
  // assembles; it does not wire.
  if (
    !isTest &&
    /plugins\/builtin\/(?!.+\/adapters\/).+\.(ts|tsx)$/.test(rel) &&
    /from\s+["']@\/main\/container["']/.test(text)
  ) {
    violations.push({
      file: rel,
      reason: "only an adapter may reach the composition root; a root assembles",
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

reportDuplicateVocabulary();

if (violations.length > 0) {
  console.error(`[check-published-boundaries] Found ${violations.length} violation(s):`);
  for (const violation of violations) console.error(`  ${violation.file}: ${violation.reason}`);
  process.exit(1);
}

console.log("[check-published-boundaries] OK — published boundaries stay wire-free.");
