// The Runtime-owned vocabulary a generated wire check is written in.
//
// Protocol §11.3 asks for a terminal runtime validator on the client side, and
// TypeScript's types are erased long before a frame arrives — so without this,
// nothing about an inbound result or event is checked at all. The rules themselves
// are NOT here: they are compiled out of the published schema into
// `wire.validate.generated.ts`. This file says what each JSON Schema keyword MEANS,
// once, exactly as `required` / `positive` / `oneOf` stay hand-written beside the
// generated Go validators.
//
// Two deliberate silences, both of them agreement with the published schema rather
// than an opinion of this file:
//
//   - An unknown property is not a violation. The runtime's decoder ignores unknown
//     fields, so the bundle carries no `additionalProperties: false`; refusing them
//     here would reject frames the runtime accepts.
//   - `format` and `contentEncoding` are annotations, not assertions — the default
//     reading of both keywords. A malformed timestamp is caught where it is decoded.

/** One rule a value broke, and where. */
export interface WireViolation {
  /**
   * Rooted at the validated type's name, so a report reads
   * `RunEvent.event.outcome.type`. An array element carries its index.
   */
  path: string;
  detail: string;
}

/**
 * A check appends every violation it finds to `out` rather than returning at the
 * first — a caller fixing a frame wants the whole list, and `oneOf` needs to count
 * how badly each variant missed.
 */
export type WireCheck = (value: unknown, path: string, out: WireViolation[]) => void;

/** `type: "object"` together with the `properties` / `required` it carries. */
export function object(
  properties: Record<string, WireCheck>,
  required: readonly string[],
): WireCheck {
  const stated = fields(properties, required);
  return (value, path, out) => {
    if (!isObject(value)) {
      out.push({ path, detail: "expected an object" });
      return;
    }
    stated(value, path, out);
  };
}

/**
 * `properties` and `required` WITHOUT asserting the value is an object — the
 * faithful reading of both keywords, and the one a union branch or a presence rule
 * needs: the enclosing definition already asserted the type, and a branch that
 * re-asserted it would report the same wrong type once per variant.
 */
export function fields(
  properties: Record<string, WireCheck>,
  required: readonly string[],
): WireCheck {
  return (value, path, out) => {
    if (!isObject(value)) return;
    for (const name of required) {
      if (value[name] === undefined) out.push({ path: `${path}.${name}`, detail: "is required" });
    }
    for (const [name, check] of Object.entries(properties)) {
      if (value[name] !== undefined) check(value[name], `${path}.${name}`, out);
    }
  };
}

/**
 * The boolean schema `false` on a property: this variant may not carry the field.
 * `fields` only reaches a property that is present, so this fires exactly then.
 */
export function absent(): WireCheck {
  return (_value, path, out) => out.push({ path, detail: "must not be present here" });
}

export function text(): WireCheck {
  return (value, path, out) => {
    if (typeof value !== "string") out.push({ path, detail: "expected a string" });
  };
}

export function integer(): WireCheck {
  return (value, path, out) => {
    if (typeof value !== "number" || !Number.isInteger(value)) {
      out.push({ path, detail: "expected an integer" });
    }
  };
}

export function numeric(): WireCheck {
  return (value, path, out) => {
    if (typeof value !== "number" || !Number.isFinite(value)) {
      out.push({ path, detail: "expected a number" });
    }
  };
}

/**
 * `minLength`, which constrains a string and says nothing about anything else.
 *
 * It is its own check rather than an argument to `text` because the schema states
 * it BOTH ways: beside a type keyword on a request's own field, and alone in the
 * allOf branch that constrains a field of a shared shape. One rule reads as one
 * call either way.
 */
export function minLength(least: number): WireCheck {
  return (value, path, out) => {
    if (typeof value === "string" && Array.from(value).length < least) {
      out.push({ path, detail: `expected at least ${least} character(s)` });
    }
  };
}

/** `maxLength`, counted as Unicode code points as required by JSON Schema. */
export function maxLength(most: number): WireCheck {
  return (value, path, out) => {
    if (typeof value === "string" && Array.from(value).length > most) {
      out.push({ path, detail: `expected at most ${most} character(s)` });
    }
  };
}

/** `pattern`, applied only to strings as JSON Schema specifies. */
export function pattern(expression: string): WireCheck {
  const compiled = new RegExp(expression);
  return (value, path, out) => {
    if (typeof value === "string" && !compiled.test(value)) {
      out.push({ path, detail: `expected to match ${expression}` });
    }
  };
}

/** `minimum`, which constrains a number and says nothing about anything else. */
export function minimum(least: number): WireCheck {
  return (value, path, out) => {
    if (typeof value === "number" && value < least) {
      out.push({ path, detail: `expected at least ${least}` });
    }
  };
}

/** `maximum`, which constrains a number and says nothing about anything else. */
export function maximum(most: number): WireCheck {
  return (value, path, out) => {
    if (typeof value === "number" && value > most) {
      out.push({ path, detail: `expected at most ${most}` });
    }
  };
}

/** `minItems`, which constrains an array's length and says nothing about its elements. */
export function minItems(least: number): WireCheck {
  return (value, path, out) => {
    if (Array.isArray(value) && value.length < least) {
      out.push({ path, detail: `expected at least ${least} item(s)` });
    }
  };
}

/** `minProperties`, which constrains an object's own-property count. */
export function minProperties(least: number): WireCheck {
  return (value, path, out) => {
    if (
      typeof value === "object" &&
      value !== null &&
      !Array.isArray(value) &&
      Object.keys(value).length < least
    ) {
      const noun = least === 1 ? "property" : "properties";
      out.push({ path, detail: `expected at least ${least} ${noun}` });
    }
  };
}

/**
 * `uniqueItems`. Elements are compared by their JSON text, which is what the
 * keyword means: two array entries are the same item when they are the same value,
 * not when they are the same object.
 */
export function uniqueItems(): WireCheck {
  return (value, path, out) => {
    if (!Array.isArray(value)) return;
    const seen = new Set<string>();
    for (const element of value) {
      const key = JSON.stringify(element) ?? "undefined";
      if (seen.has(key)) {
        out.push({ path, detail: "expected no repeated items" });
        return;
      }
      seen.add(key);
    }
  };
}

export function flag(): WireCheck {
  return (value, path, out) => {
    if (typeof value !== "boolean") out.push({ path, detail: "expected a boolean" });
  };
}

/** `const`: the discriminator a union branch pins. */
export function literal(expected: string): WireCheck {
  return (value, path, out) => {
    if (value !== expected) out.push({ path, detail: `expected ${JSON.stringify(expected)}` });
  };
}

/**
 * `enum`. The value set implies the string type keyword it sits beside, so a
 * non-string is reported once — as "not one of these" rather than twice.
 */
export function enumOf(values: readonly string[]): WireCheck {
  return (value, path, out) => {
    if (typeof value !== "string" || !values.includes(value)) {
      out.push({
        path,
        detail: `expected one of ${values.map((v) => JSON.stringify(v)).join(", ")}`,
      });
    }
  };
}

/** The empty schema: an opaque passthrough carries any JSON value, by design. */
export function anything(): WireCheck {
  return () => {};
}

/** A nullable result: JSON null or the declared non-null shape. */
export function nullable(value: WireCheck): WireCheck {
  return (candidate, path, out) => {
    if (candidate !== null) value(candidate, path, out);
  };
}

export function array(items: WireCheck): WireCheck {
  return (value, path, out) => {
    if (!Array.isArray(value)) {
      out.push({ path, detail: "expected an array" });
      return;
    }
    value.forEach((element, index) => items(element, `${path}[${index}]`, out));
  };
}

/** `additionalProperties`: a map keyed by any string. */
export function record(values: WireCheck): WireCheck {
  return (value, path, out) => {
    if (!isObject(value)) {
      out.push({ path, detail: "expected an object" });
      return;
    }
    for (const [key, member] of Object.entries(value)) {
      values(member, `${path}.${key}`, out);
    }
  };
}

/**
 * `$ref`. The target is resolved on each call, not captured: the definitions
 * reference one another (an item carries content blocks, a block carries items),
 * and a value captured at construction time would be undefined for whichever half
 * of a cycle is built second.
 */
export function ref(resolve: () => WireCheck): WireCheck {
  return (value, path, out) => resolve()(value, path, out);
}

/**
 * `oneOf`: exactly one variant applies.
 *
 * The verdict is exact. The report is a heuristic — when nothing matched, the
 * nearest miss is appended, so a caller reads "outcome.result is required" instead
 * of only "no variant matched". Matching MORE than one variant is a defect in the
 * union's exclusivity rather than in the frame, and says so.
 */
export function oneOf(branches: readonly WireCheck[]): WireCheck {
  return (value, path, out) => {
    let matched = 0;
    let nearest: WireViolation[] | undefined;
    let nearestMissesDiscriminator = true;
    for (const branch of branches) {
      const missed: WireViolation[] = [];
      branch(value, path, missed);
      if (missed.length === 0) {
        matched++;
        continue;
      }
      // Every first-party union is discriminated by `type`. Once that tag matches,
      // report the defects in that variant even when an earlier wrong-tag branch
      // happens to have fewer violations. Otherwise `{type:"resync"}` misleadingly
      // reports `expected "skills.changed"` instead of `topics is required`.
      const missesDiscriminator = missed.some((violation) => violation.path === `${path}.type`);
      if (
        nearest === undefined ||
        (nearestMissesDiscriminator && !missesDiscriminator) ||
        (nearestMissesDiscriminator === missesDiscriminator && missed.length < nearest.length)
      ) {
        nearest = missed;
        nearestMissesDiscriminator = missesDiscriminator;
      }
    }
    if (matched === 1) return;
    if (matched > 1) {
      out.push({ path, detail: `matches ${matched} variants, and exactly one may apply` });
      return;
    }
    out.push({ path, detail: "matches no permitted variant" });
    if (nearest !== undefined) out.push(...nearest);
  };
}

export function allOf(members: readonly WireCheck[]): WireCheck {
  return (value, path, out) => {
    for (const member of members) member(value, path, out);
  };
}

/** `if` / `then`: a cross-field presence rule. */
export function ifThen(condition: WireCheck, consequence: WireCheck): WireCheck {
  return (value, path, out) => {
    const unmet: WireViolation[] = [];
    condition(value, path, unmet);
    if (unmet.length === 0) consequence(value, path, out);
  };
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
