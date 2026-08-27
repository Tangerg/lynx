import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

const script = readFileSync(new URL("./check-backend-api-consumers.mjs", import.meta.url), "utf8");
const discardStart = script.indexOf("function discardsNonVoidResult(call)");
const discardEnd = script.indexOf("\nfunction propertyName", discardStart);
if (discardStart < 0 || discardEnd < 0) throw new Error("discarded-result helpers were not found");
const discardedResultHelpers = script.slice(discardStart, discardEnd);
const sidecarMatch =
  /function checkSidecarConsumers\(expected, methods, calls, targetErrors\) \{[\s\S]*?\n\}/.exec(
    script,
  );
if (!sidecarMatch) throw new Error("checkSidecarConsumers helper was not found");
const materializedMatch =
  /function creditMaterializedOperationConsumers\(methods, consumersByOperation, targetErrors\) \{[\s\S]*?\n\}/.exec(
    script,
  );
if (!materializedMatch)
  throw new Error("creditMaterializedOperationConsumers helper was not found");

function discardedResult(sourceText) {
  const dir = mkdtempSync(join(tmpdir(), "api-result-consumer-"));
  try {
    writeFileSync(join(dir, "package.json"), '{"type":"module"}\n');
    writeFileSync(join(dir, "probe.ts"), sourceText);
    writeFileSync(
      join(dir, "probe.mjs"),
      `import { API, TypeFlags } from ${JSON.stringify(import.meta.resolve("typescript/unstable/sync"))};
import * as ts from ${JSON.stringify(import.meta.resolve("typescript/unstable/ast"))};
const fileName = ${JSON.stringify(join(dir, "probe.ts"))};
const compiler = new API({ cwd: ${JSON.stringify(dir)} });
try {
  const snapshot = compiler.updateSnapshot({ openFiles: [fileName] });
  const project = snapshot.getDefaultProjectForFile(fileName);
  if (!project) throw new Error("TypeScript did not create an inferred project");
  const checker = project.checker;
  const source = project.program.getSourceFile(fileName);
  if (!source) throw new Error("TypeScript did not load the probe source");
  ${discardedResultHelpers}
  let call;
  function visit(node) { if (ts.isCallExpression(node) && node.expression.getText(source) === "saved") call = node; node.forEachChild(visit); }
  visit(source);
  console.log(discardsNonVoidResult(call));
} finally {
  compiler.close();
}
`,
    );
    const output = execFileSync(process.execPath, [join(dir, "probe.mjs")], {
      encoding: "utf8",
    }).trim();
    return output === "true";
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

test("flags direct and explicit-void discards of a non-void SDK result", () => {
  assert.equal(
    discardedResult(
      "declare function saved(): Promise<{ id: string }>; async function run() { await saved(); }",
    ),
    true,
  );
  assert.equal(
    discardedResult(
      "declare function saved(): Promise<{ id: string }>; function run() { void saved(); }",
    ),
    true,
  );
});

test("allows a statement whose awaited result is void", () => {
  assert.equal(
    discardedResult(
      "declare function saved(): Promise<void>; async function run() { await saved(); }",
    ),
    false,
  );
});

test("fails a manifest sidecar with no product callsite", () => {
  const checkSidecarConsumers = Function(
    `return (${sidecarMatch[0].replace(/^function checkSidecarConsumers/, "function")})`,
  )();
  const errors = [];
  checkSidecarConsumers(
    new Set(["info", "liveness", "readiness"]),
    new Map([
      ["info", "info"],
      ["liveness", "liveness"],
      ["readiness", "readiness"],
    ]),
    new Map([
      ["info", new Set(["runtimeServiceInspector.ts:1"])],
      ["liveness", new Set(["runtimeServiceInspector.ts:2"])],
    ]),
    errors,
  );
  assert.deepEqual(errors, [
    "readiness has no non-test frontend consumer (the SidecarClient implementation and tests do not count)",
  ]);
});

test("credits query facts only when their server composite has a product callsite", () => {
  const creditMaterializedOperationConsumers = Function(
    `return (${materializedMatch[0].replace(
      /^function creditMaterializedOperationConsumers/,
      "function",
    )})`,
  )();
  const methods = [
    { name: "items.list" },
    { name: "runs.list" },
    { name: "sessions.snapshot", materializes: ["items.list", "runs.list"] },
  ];
  const consumers = new Map([
    ["items.list", new Set()],
    ["runs.list", new Set()],
    ["sessions.snapshot", new Set(["agentRuntimeGateway.ts:1"])],
  ]);
  const errors = [];

  const credited = creditMaterializedOperationConsumers(methods, consumers, errors);

  assert.deepEqual([...credited], ["items.list", "runs.list"]);
  assert.deepEqual(
    [...consumers.get("items.list")],
    ["agentRuntimeGateway.ts:1 (materialized by sessions.snapshot)"],
  );
  assert.deepEqual(errors, []);
});
