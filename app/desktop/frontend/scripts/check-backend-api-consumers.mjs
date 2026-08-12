#!/usr/bin/env node
import { readFileSync } from "node:fs";
import { resolve, relative, sep } from "node:path";
import ts from "typescript";

const ROOT = process.cwd();
const METHODS_PATH = resolve(ROOT, "src/rpc/methods.ts");
const SIDECAR_PATH = resolve(ROOT, "src/rpc/sidecar.ts");
const MANIFEST_PATH = resolve(ROOT, "../../runtime/contract/manifest.json");
const EVENT_POLICY_PATH = resolve(
  ROOT,
  "src/plugins/builtin/workspace/events/domain/eventInvalidation.ts",
);
const EVENT_ADAPTER_PATH = resolve(
  ROOT,
  "src/plugins/builtin/workspace/events/adapters/runtimeWorkspaceEvents.ts",
);

const manifest = JSON.parse(readFileSync(MANIFEST_PATH, "utf8"));
const operations = new Set(manifest.methods.map((method) => method.name));
const sidecarEndpoints = new Set(
  manifest.httpEndpoints
    .filter((endpoint) => endpoint.kind === "sidecar")
    .map((endpoint) => endpoint.name),
);
const runtimeTopics = new Set(manifest.runtimeTopics.map((topic) => topic.type));

const configPath = ts.findConfigFile(ROOT, ts.sys.fileExists, "tsconfig.json");
if (!configPath) fail(["tsconfig.json was not found"]);
const config = ts.readConfigFile(configPath, ts.sys.readFile);
if (config.error) fail([formatDiagnostic(config.error)]);
const parsed = ts.parseJsonConfigFileContent(config.config, ts.sys, ROOT);
if (parsed.errors.length > 0) fail(parsed.errors.map(formatDiagnostic));
const program = ts.createProgram(parsed.fileNames, parsed.options);
const checker = program.getTypeChecker();
const methodsSource = program.getSourceFile(METHODS_PATH);
if (!methodsSource) fail([`TypeScript did not load ${relative(ROOT, METHODS_PATH)}`]);
const sidecarSource = program.getSourceFile(SIDECAR_PATH);
if (!sidecarSource) fail([`TypeScript did not load ${relative(ROOT, SIDECAR_PATH)}`]);

const implementationMap = new Map();
mapReturnedObject("createMethods", []);
mapReturnedObject("bindWorkspace", []);
const sidecarMethodMap = mappedSidecarMethods();

const consumerCalls = new Map();
const sidecarConsumerCalls = new Map();
const discardedResults = [];
for (const source of program.getSourceFiles()) {
  const sourcePath = resolve(source.fileName);
  if (!isProductSource(sourcePath)) continue;
  visit(source, (node) => {
    if (!ts.isCallExpression(node) || !ts.isPropertyAccessExpression(node.expression)) return;
    let symbol = checker.getSymbolAtLocation(node.expression.name);
    if (!symbol) return;
    if (symbol.flags & ts.SymbolFlags.Alias) symbol = checker.getAliasedSymbol(symbol);
    for (const declaration of symbol.declarations ?? []) {
      const declarationPath = resolve(declaration.getSourceFile().fileName);
      let wrapper;
      let target;
      if (declarationPath === METHODS_PATH) {
        wrapper = wrapperPath(declaration, METHODS_PATH);
        target = consumerCalls;
      } else if (declarationPath === SIDECAR_PATH) {
        wrapper = wrapperPath(declaration, SIDECAR_PATH);
        target = sidecarConsumerCalls;
      } else {
        continue;
      }
      if (!wrapper || !target) continue;
      const position = source.getLineAndCharacterOfPosition(node.getStart(source));
      const location = `${relative(ROOT, sourcePath)}:${position.line + 1}`;
      const locations = target.get(wrapper) ?? new Set();
      locations.add(location);
      target.set(wrapper, locations);
      if (discardsNonVoidResult(node)) {
        discardedResults.push(`${wrapper} returns a value that is discarded at ${location}`);
      }
    }
  });
}

const errors = [];
errors.push(...discardedResults);
const operationConsumers = new Map([...operations].map((operation) => [operation, new Set()]));
for (const [wrapper, locations] of consumerCalls) {
  const mappedOperations = implementationMap.get(wrapper);
  if (!mappedOperations?.size) {
    errors.push(
      `frontend calls Methods.${wrapper}, but its implementation could not be mapped to a Runtime operation (${[...locations].join(", ")})`,
    );
    continue;
  }
  for (const operation of mappedOperations) {
    const consumers = operationConsumers.get(operation);
    if (!consumers) {
      errors.push(
        `Methods.${wrapper} maps to ${operation}, which is absent from the Runtime manifest`,
      );
      continue;
    }
    for (const location of locations) consumers.add(location);
  }
}

for (const [operation, consumers] of operationConsumers) {
  if (consumers.size === 0) {
    errors.push(
      `${operation} has no non-test frontend consumer (generated RPC wrappers and visual fixtures do not count)`,
    );
  }
}

checkSidecarConsumers(sidecarEndpoints, sidecarMethodMap, sidecarConsumerCalls, errors);

checkRuntimeTopics(errors);
if (errors.length > 0) fail(errors);

const callCount =
  [...consumerCalls.values()].reduce((total, locations) => total + locations.size, 0) +
  [...sidecarConsumerCalls.values()].reduce((total, locations) => total + locations.size, 0);
console.log(
  `check-backend-api-consumers: ${operations.size}/${operations.size} Runtime operations, ${sidecarEndpoints.size}/${sidecarEndpoints.size} HTTP sidecars, and ${runtimeTopics.size}/${runtimeTopics.size} event types have product consumers (${callCount} typed call sites)`,
);

function mappedSidecarMethods() {
  let declaration;
  for (const statement of sidecarSource.statements) {
    if (!ts.isVariableStatement(statement)) continue;
    declaration = statement.declarationList.declarations.find(
      (candidate) => ts.isIdentifier(candidate.name) && candidate.name.text === "SIDECAR_METHODS",
    );
    if (declaration) break;
  }
  const initializer = declaration?.initializer && unwrapExpression(declaration.initializer);
  if (!initializer || !ts.isObjectLiteralExpression(initializer)) {
    fail(["SIDECAR_METHODS must be an object literal in sidecar.ts"]);
  }
  const mapped = new Map();
  for (const property of initializer.properties) {
    if (!ts.isPropertyAssignment(property)) continue;
    const endpoint = propertyName(property.name);
    const method = constantString(property.initializer);
    if (endpoint && method) mapped.set(endpoint, method);
  }
  return mapped;
}

function checkSidecarConsumers(expected, methods, calls, targetErrors) {
  for (const endpoint of expected) {
    const method = methods.get(endpoint);
    if (!method) {
      targetErrors.push(`${endpoint} has no typed SidecarClient method mapping`);
      continue;
    }
    const consumers = calls.get(method);
    if (!consumers?.size) {
      targetErrors.push(
        `${endpoint} has no non-test frontend consumer (the SidecarClient implementation and tests do not count)`,
      );
    }
  }
  for (const endpoint of methods.keys()) {
    if (!expected.has(endpoint)) {
      targetErrors.push(
        `SidecarClient maps ${endpoint}, which is absent from the Runtime manifest`,
      );
    }
  }
}

function mapReturnedObject(functionName, prefix) {
  const declaration = methodsSource.statements.find(
    (statement) => ts.isFunctionDeclaration(statement) && statement.name?.text === functionName,
  );
  if (!declaration?.body) fail([`${functionName} implementation was not found in methods.ts`]);
  const returned = declaration.body.statements.find(
    (statement) =>
      ts.isReturnStatement(statement) && ts.isObjectLiteralExpression(statement.expression),
  );
  if (!returned || !ts.isReturnStatement(returned) || !returned.expression) {
    fail([`${functionName} must directly return its Methods object`]);
  }
  walkImplementationObject(returned.expression, prefix);
}

function walkImplementationObject(object, prefix) {
  for (const property of object.properties) {
    if (!ts.isPropertyAssignment(property)) continue;
    const name = propertyName(property.name);
    if (!name) continue;
    const path = [...prefix, name];
    if (ts.isObjectLiteralExpression(property.initializer)) {
      walkImplementationObject(property.initializer, path);
      continue;
    }
    const mapped = wireOperationsIn(property.initializer);
    if (mapped.size === 0) continue;
    const key = path.join(".");
    const current = implementationMap.get(key) ?? new Set();
    for (const operation of mapped) current.add(operation);
    implementationMap.set(key, current);
  }
}

function wireOperationsIn(initializer) {
  const found = new Set();
  const scannedInitializers = new Set();

  const scan = (node) => {
    if (ts.isCallExpression(node) && node.arguments.length > 0) {
      const operation = constantString(node.arguments[0]);
      if (operation && operations.has(operation)) found.add(operation);
    }
    ts.forEachChild(node, scan);
  };

  const scanInitializer = (node) => {
    if (ts.isIdentifier(node)) {
      let symbol = checker.getSymbolAtLocation(node);
      if (symbol?.flags & ts.SymbolFlags.Alias) symbol = checker.getAliasedSymbol(symbol);
      for (const declaration of symbol?.declarations ?? []) {
        if (!ts.isVariableDeclaration(declaration) || !declaration.initializer) continue;
        if (scannedInitializers.has(declaration)) continue;
        scannedInitializers.add(declaration);
        scan(declaration.initializer);
      }
    }
    scan(node);
  };

  scanInitializer(initializer);
  return found;
}

function constantString(expression) {
  const node = unwrapExpression(expression);
  if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) return node.text;
  if (!ts.isIdentifier(node)) return undefined;
  let symbol = checker.getSymbolAtLocation(node);
  if (symbol?.flags & ts.SymbolFlags.Alias) symbol = checker.getAliasedSymbol(symbol);
  for (const declaration of symbol?.declarations ?? []) {
    if (ts.isVariableDeclaration(declaration) && declaration.initializer) {
      const value = constantString(declaration.initializer);
      if (value !== undefined) return value;
    }
  }
  return undefined;
}

function unwrapExpression(expression) {
  let node = expression;
  while (
    ts.isParenthesizedExpression(node) ||
    ts.isAsExpression(node) ||
    ts.isSatisfiesExpression(node) ||
    ts.isTypeAssertionExpression(node)
  ) {
    node = node.expression;
  }
  return node;
}

function wrapperPath(declaration, declarationSourcePath) {
  const parts = [];
  let node = declaration;
  while (node && resolve(node.getSourceFile().fileName) === declarationSourcePath) {
    if (
      (ts.isPropertySignature(node) ||
        ts.isPropertyDeclaration(node) ||
        ts.isMethodSignature(node)) &&
      propertyName(node.name)
    ) {
      const name = propertyName(node.name);
      if (parts[0] !== name) parts.unshift(name);
    }
    if (ts.isInterfaceDeclaration(node)) break;
    node = node.parent;
  }
  return parts.join(".");
}

function discardsNonVoidResult(call) {
  let expression = call;
  while (
    ts.isAwaitExpression(expression.parent) ||
    ts.isParenthesizedExpression(expression.parent) ||
    ts.isAsExpression(expression.parent) ||
    ts.isSatisfiesExpression(expression.parent) ||
    ts.isTypeAssertionExpression(expression.parent) ||
    ts.isNonNullExpression(expression.parent) ||
    ts.isVoidExpression(expression.parent)
  ) {
    expression = expression.parent;
  }
  if (!ts.isExpressionStatement(expression.parent)) return false;
  const result = checker.getAwaitedType(checker.getTypeAtLocation(call));
  return !result || (result.flags & (ts.TypeFlags.Void | ts.TypeFlags.Never)) === 0;
}

function propertyName(node) {
  if (ts.isIdentifier(node) || ts.isStringLiteral(node) || ts.isNumericLiteral(node)) {
    return node.text;
  }
  return undefined;
}

function isProductSource(sourcePath) {
  const sourceRoot = `${sep}src${sep}`;
  if (!sourcePath.includes(sourceRoot)) return false;
  if (sourcePath.includes(`${sourceRoot}rpc${sep}`)) return false;
  if (/\.(?:test|spec)\.[cm]?[jt]sx?$/.test(sourcePath)) return false;
  if (/\.generated\.[cm]?[jt]s$/.test(sourcePath)) return false;
  return true;
}

function checkRuntimeTopics(targetErrors) {
  const policy = ts.createSourceFile(
    EVENT_POLICY_PATH,
    readFileSync(EVENT_POLICY_PATH, "utf8"),
    ts.ScriptTarget.Latest,
    true,
    ts.ScriptKind.TS,
  );
  const adapter = ts.createSourceFile(
    EVENT_ADAPTER_PATH,
    readFileSync(EVENT_ADAPTER_PATH, "utf8"),
    ts.ScriptTarget.Latest,
    true,
    ts.ScriptKind.TS,
  );

  const ownedTypes = stringUnion(policy, "WorkspaceEventType");
  compareSets("WorkspaceEventType", runtimeTopics, ownedTypes, targetErrors);

  const handled = new Set();
  visit(policy, (node) => {
    if (ts.isCaseClause(node) && node.expression && ts.isStringLiteral(node.expression)) {
      if (runtimeTopics.has(node.expression.text)) handled.add(node.expression.text);
    }
  });
  compareSets("workspaceInvalidations switch", runtimeTopics, handled, targetErrors);

  const subscribed = stringArray(adapter, "SUBSCRIBED_TOPICS");
  const subscribable = new Set([...runtimeTopics].filter((topic) => topic !== "resync"));
  compareSets("SUBSCRIBED_TOPICS", subscribable, subscribed, targetErrors);
}

function stringUnion(source, name) {
  const declaration = source.statements.find(
    (statement) => ts.isTypeAliasDeclaration(statement) && statement.name.text === name,
  );
  if (!declaration || !ts.isUnionTypeNode(declaration.type)) return new Set();
  return new Set(
    declaration.type.types.flatMap((type) =>
      ts.isLiteralTypeNode(type) && ts.isStringLiteral(type.literal) ? [type.literal.text] : [],
    ),
  );
}

function stringArray(source, name) {
  for (const statement of source.statements) {
    if (!ts.isVariableStatement(statement)) continue;
    for (const declaration of statement.declarationList.declarations) {
      if (!ts.isIdentifier(declaration.name) || declaration.name.text !== name) continue;
      const initializer = declaration.initializer && unwrapExpression(declaration.initializer);
      if (!initializer || !ts.isArrayLiteralExpression(initializer)) return new Set();
      return new Set(
        initializer.elements.flatMap((element) =>
          ts.isStringLiteral(element) ? [element.text] : [],
        ),
      );
    }
  }
  return new Set();
}

function compareSets(label, expected, actual, targetErrors) {
  const missing = [...expected].filter((value) => !actual.has(value));
  const extra = [...actual].filter((value) => !expected.has(value));
  if (missing.length > 0) targetErrors.push(`${label} is missing: ${missing.join(", ")}`);
  if (extra.length > 0) targetErrors.push(`${label} has non-manifest values: ${extra.join(", ")}`);
}

function visit(node, callback) {
  callback(node);
  ts.forEachChild(node, (child) => visit(child, callback));
}

function formatDiagnostic(diagnostic) {
  return ts.flattenDiagnosticMessageText(diagnostic.messageText, "\n");
}

function fail(messages) {
  console.error("[check-backend-api-consumers] Failed:");
  for (const message of messages) console.error(`  - ${message}`);
  process.exit(1);
}
