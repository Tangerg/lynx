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
    !rel.startsWith("plugins/sdk/") &&
    !rel.startsWith("plugins/host/") &&
    rel !== "plugins/builtin/agent/public/viewState.ts" &&
    /from\s+["']@\/plugins\/sdk\/types\/agent(View|Timeline)["']/.test(text)
  ) {
    violations.push({
      file: rel,
      reason: "agent view language must be consumed through agent public/viewState",
    });
  }

  if (
    !isTest &&
    /plugins\/builtin\/.+\/domain\/.+\.(ts|tsx)$/.test(rel) &&
    /from\s+["'](?:react|zustand(?:\/[^"']*)?|@\/(?:rpc|state|main|components|pages)(?:\/[^"']*)?)["']/.test(
      text,
    )
  ) {
    violations.push({
      file: rel,
      reason: "builtin context domain must stay framework-, store-, wire-, and UI-free",
    });
  }

  if (
    !isTest &&
    /plugins\/builtin\/.+\/application\/.+\.(ts|tsx)$/.test(rel) &&
    /from\s+["']@\/(?:components|pages)(?:\/[^"']*)?["']/.test(text)
  ) {
    violations.push({
      file: rel,
      reason: "builtin context application must not import UI components or pages",
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
    /plugins\/builtin\/.+\/public\/.+\.(ts|tsx)$/.test(rel) &&
    !/plugins\/builtin\/.+\/public\/statePorts\.ts$/.test(rel) &&
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
