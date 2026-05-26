// Tiny expression evaluator for command `when` clauses (VS Code-style,
// stripped down). Grammar:
//   or → and ("||" and)*    eq → unary (("==" | "!=") unary)?
//   and → eq ("&&" eq)*     unary → "!" unary | primary
//   primary → identifier | string | "(" or ")"
// Parse errors / unknown identifiers collapse to `false` — better to
// hide an entry than throw at palette-render time.

type Token =
  | { type: "id"; value: string }
  | { type: "str"; value: string }
  | { type: "op"; value: "!" | "==" | "!=" | "&&" | "||" | "(" | ")" };

type Node =
  | { kind: "id"; name: string }
  | { kind: "str"; value: string }
  | { kind: "not"; child: Node }
  | { kind: "eq" | "neq" | "and" | "or"; left: Node; right: Node };

export type WhenContext = Record<string, unknown>;

const ID_CHAR = /[\w.]/;

function tokenize(s: string): Token[] {
  const out: Token[] = [];
  let i = 0;
  while (i < s.length) {
    const c = s[i];
    if (/\s/.test(c)) {
      i++;
      continue;
    }
    if (c === "(" || c === ")") {
      out.push({ type: "op", value: c });
      i++;
      continue;
    }
    if (s.startsWith("==", i)) {
      out.push({ type: "op", value: "==" });
      i += 2;
      continue;
    }
    if (s.startsWith("!=", i)) {
      out.push({ type: "op", value: "!=" });
      i += 2;
      continue;
    }
    if (s.startsWith("&&", i)) {
      out.push({ type: "op", value: "&&" });
      i += 2;
      continue;
    }
    if (s.startsWith("||", i)) {
      out.push({ type: "op", value: "||" });
      i += 2;
      continue;
    }
    if (c === "!") {
      out.push({ type: "op", value: "!" });
      i++;
      continue;
    }
    if (c === '"' || c === "'") {
      const end = s.indexOf(c, i + 1);
      if (end < 0) throw new Error(`unclosed string literal`);
      out.push({ type: "str", value: s.slice(i + 1, end) });
      i = end + 1;
      continue;
    }
    if (ID_CHAR.test(c)) {
      let j = i;
      while (j < s.length && ID_CHAR.test(s[j])) j++;
      out.push({ type: "id", value: s.slice(i, j) });
      i = j;
      continue;
    }
    throw new Error(`unexpected character "${c}"`);
  }
  return out;
}

class Parser {
  private pos = 0;
  constructor(private readonly tokens: Token[]) {}

  parse(): Node {
    const node = this.parseOr();
    if (this.pos < this.tokens.length) {
      throw new Error(`unexpected token "${this.tokens[this.pos].value}"`);
    }
    return node;
  }

  private parseOr(): Node {
    let left = this.parseAnd();
    while (this.peekOp("||")) {
      this.consume();
      left = { kind: "or", left, right: this.parseAnd() };
    }
    return left;
  }

  private parseAnd(): Node {
    let left = this.parseEq();
    while (this.peekOp("&&")) {
      this.consume();
      left = { kind: "and", left, right: this.parseEq() };
    }
    return left;
  }

  private parseEq(): Node {
    const left = this.parseUnary();
    if (this.peekOp("==") || this.peekOp("!=")) {
      const op = this.consume().value as "==" | "!=";
      const right = this.parseUnary();
      return { kind: op === "==" ? "eq" : "neq", left, right };
    }
    return left;
  }

  private parseUnary(): Node {
    if (this.peekOp("!")) {
      this.consume();
      return { kind: "not", child: this.parseUnary() };
    }
    return this.parsePrimary();
  }

  private parsePrimary(): Node {
    const t = this.consume();
    if (t.type === "op" && t.value === "(") {
      const inner = this.parseOr();
      if (!this.peekOp(")")) throw new Error('expected ")"');
      this.consume();
      return inner;
    }
    if (t.type === "id") return { kind: "id", name: t.value };
    if (t.type === "str") return { kind: "str", value: t.value };
    throw new Error(`unexpected token "${t.value}"`);
  }

  private peekOp(v: string): boolean {
    const t = this.tokens[this.pos];
    return t !== undefined && t.type === "op" && t.value === v;
  }

  private consume(): Token {
    const t = this.tokens[this.pos];
    if (!t) throw new Error("unexpected end of expression");
    this.pos++;
    return t;
  }
}

function evaluate(node: Node, ctx: WhenContext): unknown {
  switch (node.kind) {
    case "id":
      return ctx[node.name];
    case "str":
      return node.value;
    case "not":
      return !evaluate(node.child, ctx);
    case "eq":
      return evaluate(node.left, ctx) === evaluate(node.right, ctx);
    case "neq":
      return evaluate(node.left, ctx) !== evaluate(node.right, ctx);
    case "and":
      return !!evaluate(node.left, ctx) && !!evaluate(node.right, ctx);
    case "or":
      return !!evaluate(node.left, ctx) || !!evaluate(node.right, ctx);
  }
}

// Tiny parse-AST cache. Most when clauses are static module-level strings,
// so caching by source avoids re-parsing on every render.
const cache = new Map<string, Node | "ERROR">();

export function evalWhen(expr: string, ctx: WhenContext): boolean {
  let node = cache.get(expr);
  if (node === undefined) {
    try {
      node = new Parser(tokenize(expr)).parse();
    } catch (err) {
      console.warn(`[when] failed to parse "${expr}":`, err);
      node = "ERROR";
    }
    cache.set(expr, node);
  }
  if (node === "ERROR") return false;
  return !!evaluate(node, ctx);
}
