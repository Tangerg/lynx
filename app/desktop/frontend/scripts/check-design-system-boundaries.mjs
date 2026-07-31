#!/usr/bin/env node

import { readFileSync, readdirSync, statSync } from "node:fs";
import { extname, relative } from "node:path";
import ts from "typescript";

const SRC = new URL("../src/", import.meta.url).pathname;
const PRIMITIVES = "ui/primitives/";
const DESIGN_SYSTEM_RINGS = ["ui/primitives/", "ui/atoms/", "ui/agent/"];
const NATIVE_INTERACTIVE_TAGS = new Set([
  "a",
  "button",
  "details",
  "input",
  "select",
  "summary",
  "textarea",
]);
const NATIVE_INTERACTIVE_ROLES = new Set([
  "button",
  "checkbox",
  "menuitem",
  "option",
  "radio",
  "separator",
  "slider",
  "switch",
  "tab",
  "treeitem",
]);

function* walk(dir) {
  for (const entry of readdirSync(dir)) {
    const path = `${dir}${entry}`;
    if (statSync(path).isDirectory()) yield* walk(`${path}/`);
    else yield path;
  }
}

function isTestFile(path) {
  return /\.(?:spec|test)\.[jt]sx?$/.test(path) || path.includes("/__tests__/");
}

function lineOf(sourceFile, node) {
  return sourceFile.getLineAndCharacterOfPosition(node.getStart(sourceFile)).line + 1;
}

function stringAttribute(node, name) {
  for (const property of node.attributes.properties) {
    if (!ts.isJsxAttribute(property) || property.name.text !== name) continue;
    return property.initializer && ts.isStringLiteral(property.initializer)
      ? property.initializer.text
      : undefined;
  }
  return undefined;
}

const violations = [];

for (const path of walk(SRC)) {
  if (![".ts", ".tsx"].includes(extname(path)) || isTestFile(path)) continue;

  const rel = relative(SRC, path);
  const sourceFile = ts.createSourceFile(
    path,
    readFileSync(path, "utf8"),
    ts.ScriptTarget.Latest,
    true,
    path.endsWith(".tsx") ? ts.ScriptKind.TSX : ts.ScriptKind.TS,
  );
  const insidePrimitives = rel.startsWith(PRIMITIVES);
  const insideDesignSystem = DESIGN_SYSTEM_RINGS.some((prefix) => rel.startsWith(prefix));

  function visit(node) {
    if (ts.isImportDeclaration(node) && ts.isStringLiteral(node.moduleSpecifier)) {
      const specifier = node.moduleSpecifier.text;
      if (specifier.startsWith("@base-ui/react") && !insidePrimitives) {
        violations.push(`${rel}:${lineOf(sourceFile, node)} imports Base UI outside ui/primitives`);
      }
      if (
        (specifier === "@/ui/primitives" || specifier.startsWith("@/ui/primitives/")) &&
        !insideDesignSystem
      ) {
        violations.push(
          `${rel}:${lineOf(sourceFile, node)} imports ui/primitives outside the design system`,
        );
      }
    }

    if (
      (ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node)) &&
      ts.isIdentifier(node.tagName) &&
      node.tagName.text === node.tagName.text.toLowerCase()
    ) {
      const tag = node.tagName.text;
      if (NATIVE_INTERACTIVE_TAGS.has(tag) && !insidePrimitives) {
        violations.push(
          `${rel}:${lineOf(sourceFile, node)} renders native <${tag}> outside ui/primitives`,
        );
      }

      const role = stringAttribute(node, "role");
      if (role && NATIVE_INTERACTIVE_ROLES.has(role) && !insidePrimitives) {
        violations.push(
          `${rel}:${lineOf(sourceFile, node)} implements role="${role}" outside ui/primitives`,
        );
      }
    }

    ts.forEachChild(node, visit);
  }

  visit(sourceFile);
}

if (violations.length > 0) {
  console.error(`check-design-system-boundaries: ${violations.length} abstraction bypass(es)\n`);
  for (const violation of violations) console.error(`  ${violation}`);
  process.exit(1);
}

console.log(
  "check-design-system-boundaries: native interaction and Base UI stay behind design-system rings",
);
