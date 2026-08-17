#!/usr/bin/env node
// Port-surface guard — every method a `*Port` interface and every configured singleton
// accessor must have a production caller.
//
// WHY THIS EXISTS, when `knip` already hunts dead code: knip cannot see this one.
// A port method is implemented as an exported function in the adapter and handed to
// `configure*Port({ ... })` by name, so the adapter's own import keeps that function
// "used" for reachability analysis while nothing in the app ever calls it. Two methods
// on AgentSessionViewPort lived exactly that way — an abandoned parallel path for
// session usage, wired into the composition root, dead for as long as the live path
// (a query hook) has existed, and invisible to the gate whose whole job is finding
// dead code. The wiring is the camouflage.
//
// It matters more for a port than for an ordinary function. A port is a CONTRACT: the
// declaration says "this is what the layer below owes the layer above", and a clause
// nobody invokes is a promise about a need that does not exist. It also has to be paid
// for by every implementation, including the visual fixtures that install their own.
//
// Method of detection — a clause counts as called when either shape appears in src
// outside the port's own declaration, in a non-test file:
//
//   * a member access, `viewPort.method` / `accessor().method` — matched WITHOUT the
//     paren, because a generic call site (`.useSharedState<T>(path)`) puts a type
//     argument between the name and it. Requiring the paren reported that clause dead
//     while two modules were calling it.
//   * a destructuring binding, `const { isAvailable } = fontAvailability()` — some
//     consumers lift the clause out before calling it, so the call site has no dot to
//     find. Assuming that never happened reported a second live clause dead.
//
// What is subtracted is the interface DECLARATIONS, not the files holding them: several
// port modules also host the accessor and a thin facade over it (`draft.ts` calls
// `port.focusDraftEnd(...)` a few lines under the interface that declares it), and a
// facade its consumers go through is a caller. Skipping whole files called two more live
// clauses dead.
//
// Tests do NOT count: a clause or accessor only a test exercises is still a contract
// the app does not need. In particular, `export const gateway = port.get` is not a
// harmless test seam: it publishes a second command path around any richer owner that
// production uses, and makes the composition root install/retire global state solely
// so an adapter test can call its private mapping directly.
//
// The rule under-reports rather than over-reports. A port method whose name collides
// with a common one (`.get(`, `.subscribe(`) will look reachable through some unrelated
// object, so this will miss it — deliberate, because a guard that fails a build wrongly
// gets deleted, and one that catches most of the class keeps paying.

import { readFileSync, readdirSync, statSync } from "node:fs";
import { extname, join, relative } from "node:path";

const SRC = new URL("../src/", import.meta.url).pathname;

function* walk(dir) {
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) yield* walk(path);
    else yield path;
  }
}

const sources = [];
for (const path of walk(SRC)) {
  if (![".ts", ".tsx"].includes(extname(path))) continue;
  sources.push({ path, rel: relative(SRC, path), text: readFileSync(path, "utf8") });
}

const isTest = (rel) =>
  /\.test\.tsx?$/.test(rel) || rel.startsWith("test/") || rel.includes("testkit");

/** Each `export interface *Port` block: its methods, and the span it occupies. */
function portsIn(text) {
  const ports = [];
  const header = /export interface (\w*Port)\s*\{/g;
  let match;
  while ((match = header.exec(text)) !== null) {
    let depth = 1;
    let index = header.lastIndex;
    const methods = new Set();
    let lineStart = index;
    while (depth > 0 && index < text.length) {
      const char = text[index];
      if (char === "{") depth += 1;
      else if (char === "}") depth -= 1;
      else if (char === "\n") {
        if (depth === 1) {
          // A declaration sitting directly in the interface body: `name(` or `name<T>(`.
          const line = text.slice(lineStart, index);
          const declared = /^\s{2}(\w+)\s*[<(]/.exec(line);
          if (declared) methods.add(declared[1]);
        }
        lineStart = index + 1;
      }
      index += 1;
    }
    ports.push({ name: match[1], methods: [...methods], start: match.index, end: index });
  }
  return ports;
}

/** The file with every port interface body blanked out, so a declaration cannot pass
 *  for a call while the facade sitting beside it still can. */
function withoutPortDeclarations(text, ports) {
  let out = text;
  for (const port of ports) {
    out = out.slice(0, port.start) + " ".repeat(port.end - port.start) + out.slice(port.end);
  }
  return out;
}

// Every non-test source, with port interface bodies removed.
const searchable = sources
  .filter((file) => !isTest(file.rel))
  .map((file) => withoutPortDeclarations(file.text, portsIn(file.text)));

const dead = [];
const bypasses = [];
for (const file of sources) {
  if (isTest(file.rel)) continue;
  for (const port of portsIn(file.text)) {
    for (const method of port.methods) {
      const member = new RegExp(`\\.${method}\\b`);
      const destructured = new RegExp(`\\{[^}]*\\b${method}\\b[^}]*\\}\\s*=`);
      const called = searchable.some((text) => member.test(text) || destructured.test(text));
      if (!called) dead.push(`${file.rel}  ${port.name}.${method}()`);
    }
  }

  for (const match of file.text.matchAll(/export const (\w+) = \w+\.get;/g)) {
    const accessor = match[1];
    const call = new RegExp(`\\b${accessor}\\s*\\(`);
    if (!searchable.some((text) => call.test(text))) {
      bypasses.push(`${file.rel}  ${accessor}()`);
    }
  }
}

if (dead.length > 0) {
  console.error(`check-port-surface: ${dead.length} port clause(s) nobody calls\n`);
  for (const entry of dead) console.error(`  ${entry}`);
  console.error("");
  console.error("A port declares what the layer below owes the layer above. Delete the clause");
  console.error("and its adapter wiring, or call it. If a caller is coming, it is not yet owed.");
  process.exit(1);
}
if (bypasses.length > 0) {
  console.error(
    `check-port-surface: ${bypasses.length} singleton port accessor(s) have no product caller\n`,
  );
  for (const entry of bypasses) console.error(`  ${entry}`);
  console.error("");
  console.error("A test must enter through the same owner/facade as the product. Delete the");
  console.error("test-only accessor and redundant port installation; do not preserve a bypass.");
  process.exit(1);
}
console.log("check-port-surface: every port clause and singleton accessor has a product caller");
