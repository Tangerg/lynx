export function resultLines(result: string | undefined): string[] {
  const text = result?.trim();
  return text ? text.split("\n") : [];
}

export function parseJsonResult(result: string | undefined): Record<string, unknown> | undefined {
  if (!result) return undefined;
  try {
    const value: unknown = JSON.parse(result);
    return typeof value === "object" && value !== null && !Array.isArray(value)
      ? (value as Record<string, unknown>)
      : undefined;
  } catch {
    return undefined;
  }
}
