import { useCallback, useEffect, useRef, useState } from "react";

export const maxMemoryBytes = 8_192;

const utf8Encoder = new TextEncoder();

export function useMemoryAction() {
  const controller = useRef<AbortController | undefined>(undefined);
  const busy = useRef(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string>();

  useEffect(
    () => () => {
      const active = controller.current;
      controller.current = undefined;
      active?.abort();
    },
    [],
  );

  const run = useCallback(
    async <T,>(
      operation: (signal: AbortSignal) => Promise<T>,
    ): Promise<T | undefined> => {
      if (busy.current) return undefined;
      const request = new AbortController();
      controller.current = request;
      busy.current = true;
      setPending(true);
      setError(undefined);
      try {
        return await operation(request.signal);
      } catch (cause) {
        if (!request.signal.aborted) setError(messageOf(cause));
        return undefined;
      } finally {
        if (controller.current === request) {
          controller.current = undefined;
          busy.current = false;
          setPending(false);
        }
      }
    },
    [],
  );
  const clearError = useCallback(() => setError(undefined), []);

  return {
    pending,
    error,
    run,
    clearError,
  };
}

export function ActionError(props: { value: string | undefined }) {
  return props.value ? (
    <p className="memory-error" role="alert">
      {props.value}
    </p>
  ) : null;
}

export function MemoryByteCount(props: { value: number }) {
  const invalid = props.value > maxMemoryBytes;
  return (
    <span
      className={invalid ? "memory-byte-count invalid" : "memory-byte-count"}
    >
      {props.value.toLocaleString()} / {maxMemoryBytes.toLocaleString()} bytes
    </span>
  );
}

export function memoryBytes(value: string) {
  return utf8Encoder.encode(value).byteLength;
}

export function messageOf(error: unknown) {
  return error instanceof Error
    ? error.message
    : "Agent Memory operation failed.";
}
