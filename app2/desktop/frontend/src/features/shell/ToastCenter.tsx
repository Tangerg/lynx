import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";

import { useLocalization } from "../localization/Localization";

export type ToastTone = "info" | "success" | "error";

export interface ToastNotice {
  tone: ToastTone;
  title: string;
  detail?: string;
  durationMilliseconds?: number;
}

interface ToastRecord extends ToastNotice {
  id: number;
}

interface ToastContextValue {
  notify(notice: ToastNotice): number;
  dismiss(id: number): void;
}

const ToastContext = createContext<ToastContextValue | undefined>(undefined);
const maximumVisibleToasts = 4;

export function ToastProvider({ children }: { children: ReactNode }) {
  const nextID = useRef(0);
  const [toasts, setToasts] = useState<ToastRecord[]>([]);
  const dismiss = useCallback((id: number) => {
    setToasts((current) => current.filter((toast) => toast.id !== id));
  }, []);
  const notify = useCallback((notice: ToastNotice) => {
    const toast = { ...notice, id: ++nextID.current };
    setToasts((current) =>
      [...current, toast].slice(-maximumVisibleToasts),
    );
    return toast.id;
  }, []);
  const value = useMemo(() => ({ notify, dismiss }), [dismiss, notify]);

  return (
    <ToastContext.Provider value={value}>
      {children}
      <ToastViewport toasts={toasts} dismiss={dismiss} />
    </ToastContext.Provider>
  );
}

export function useToasts(): ToastContextValue {
  const value = useContext(ToastContext);
  if (value === undefined) {
    throw new Error("useToasts must be used within ToastProvider");
  }
  return value;
}

function ToastViewport(props: {
  toasts: readonly ToastRecord[];
  dismiss(id: number): void;
}) {
  const { t } = useLocalization();
  return (
    <section
      className="toast-viewport"
      aria-label={t("toast.region")}
    >
      {props.toasts.map((toast) => (
        <ToastItem
          key={toast.id}
          toast={toast}
          dismiss={props.dismiss}
        />
      ))}
    </section>
  );
}

function ToastItem(props: {
  toast: ToastRecord;
  dismiss(id: number): void;
}) {
  const { t } = useLocalization();
  const [paused, setPaused] = useState(false);
  useEffect(() => {
    if (paused) return;
    const timer = window.setTimeout(
      () => props.dismiss(props.toast.id),
      props.toast.durationMilliseconds ??
        (props.toast.tone === "error" ? 8_000 : 5_000),
    );
    return () => window.clearTimeout(timer);
  }, [paused, props.dismiss, props.toast]);

  return (
    <article
      className="toast-notice"
      data-tone={props.toast.tone}
      role={props.toast.tone === "error" ? "alert" : "status"}
      onPointerEnter={() => setPaused(true)}
      onPointerLeave={() => setPaused(false)}
      onFocusCapture={() => setPaused(true)}
      onBlurCapture={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget)) setPaused(false);
      }}
    >
      <span aria-hidden="true">
        {props.toast.tone === "success"
          ? "✓"
          : props.toast.tone === "error"
            ? "!"
            : "i"}
      </span>
      <div>
        <strong>{props.toast.title}</strong>
        {props.toast.detail ? <p>{props.toast.detail}</p> : null}
      </div>
      <button
        type="button"
        aria-label={t("toast.dismiss")}
        onClick={() => props.dismiss(props.toast.id)}
      >
        ×
      </button>
    </article>
  );
}
