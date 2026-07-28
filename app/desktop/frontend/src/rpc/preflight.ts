// The SDK's capability preflight.
//
// API.md §9 and contract §11.1 name three consumers of the capability rules — the
// dispatcher that enforces them, discovery that advertises them, and this — and
// forbid any of them keeping a second switch. The rules themselves are therefore
// read from the generated table, not restated; what lives here is how a rule's
// condition is evaluated, which mirrors the runtime's own matcher.
//
// It refuses ONLY what the server has already said it cannot do. With no
// negotiated snapshot — before discovery, or on a build where nothing installed
// one — every call goes out and the runtime stays authoritative: a client guessing
// "probably unsupported" would take away a feature the server offers.

import type { ServerCapabilities } from "./wire.generated";
import {
  WIRE_CAPABILITY_POLICY,
  type WireCapabilityCondition,
  type WireFeature,
  type WireMethodName,
} from "./wire.methods.generated";

/**
 * The features this call needs that the server did not advertise.
 *
 * Empty when the call is allowed, when the method is ungated, or when nothing has
 * been negotiated yet.
 */
export function unnegotiated(
  method: WireMethodName,
  params: unknown,
  capabilities: ServerCapabilities | null | undefined,
): WireFeature[] {
  const rules = WIRE_CAPABILITY_POLICY[method];
  if (!rules || !capabilities) return [];

  const missing: WireFeature[] = [];
  for (const rule of rules) {
    if (rule.when && !rule.when.every((condition) => matches(condition, params))) continue;
    for (const feature of rule.requires) {
      // §9: a key the server did not advertise reads as off, which is the same
      // reading the dispatcher's gate applies to its own advertised map.
      if (capabilities.features[feature]?.enabled !== true && !missing.includes(feature)) {
        missing.push(feature);
      }
    }
  }
  return missing;
}

function matches(condition: WireCapabilityCondition, params: unknown): boolean {
  const value = lookup(params, condition.field);
  if (condition.operator === "equals") return value === condition.value;
  return value !== undefined && !isEmpty(value);
}

/** Walks a dotted field path through the request. */
function lookup(params: unknown, path: string): unknown {
  let value = params;
  for (const segment of path.split(".")) {
    if (typeof value !== "object" || value === null || Array.isArray(value)) return undefined;
    value = (value as Record<string, unknown>)[segment];
  }
  return value;
}

/**
 * An explicitly empty value counts as absent, so `{ watches: [] }` asks for the
 * same thing as omitting the field and is gated the same way.
 */
function isEmpty(value: unknown): boolean {
  if (value === null || value === "" || value === false) return true;
  if (Array.isArray(value)) return value.length === 0;
  if (typeof value === "object") return Object.keys(value).length === 0;
  return false;
}
