import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

const script = readFileSync(new URL("./check-backend-api-consumers.mjs", import.meta.url), "utf8");
const match = /function discardsNonVoidResult\(call\) \{[\s\S]*?\n\}/.exec(script);
if (!match) throw new Error("discardsNonVoidResult helper was not found");
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
    writeFileSync(
      join(dir, "probe.mjs"),
      `import ts from ${JSON.stringify(import.meta.resolve("typescript"))};
const fileName = ${JSON.stringify(join(dir, "probe.ts"))};
const sourceText = ${JSON.stringify(sourceText)};
const options = { target: ts.ScriptTarget.Latest, module: ts.ModuleKind.ESNext };
const host = ts.createCompilerHost(options);
const getSourceFile = host.getSourceFile.bind(host);
const readFile = host.readFile.bind(host);
const fileExists = host.fileExists.bind(host);
host.getSourceFile = (name, languageVersion, onError, shouldCreate) => name === fileName ? ts.createSourceFile(name, sourceText, languageVersion, true, ts.ScriptKind.TS) : getSourceFile(name, languageVersion, onError, shouldCreate);
host.readFile = (name) => name === fileName ? sourceText : readFile(name);
host.fileExists = (name) => name === fileName || fileExists(name);
const program = ts.createProgram([fileName], options, host);
const checker = program.getTypeChecker();
const source = program.getSourceFile(fileName);
${match[0]}
let call;
function visit(node) { if (ts.isCallExpression(node) && node.expression.getText(source) === "saved") call = node; ts.forEachChild(node, visit); }
visit(source);
console.log(discardsNonVoidResult(call));
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
