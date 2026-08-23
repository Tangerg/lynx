import { useEffect, useId, useRef, useState } from "react";

import type { CreateSessionRequest, Session } from "@lyra/runtime-contract";

import { useLocalization } from "../localization/Localization";
import { presentRuntimeError } from "../localization/presentRuntimeError";
import { chooseDirectory } from "../../runtime/desktopBridge";
import { compactPath } from "./sessionPresentation";
import { ariaKeyShortcuts, commandByID } from "../shell/commandCatalog";

interface NewSessionMenuProps {
  pending: boolean;
  defaultWorkspace: string;
  onCreate: (request?: CreateSessionRequest) => Promise<Session>;
	onImport: () => Promise<Session | undefined>;
}

export function NewSessionMenu(props: NewSessionMenuProps) {
  const { t } = useLocalization();
  const menuId = useId();
  const root = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const [error, setError] = useState<unknown>();
  const closing = useDropdownClosing(open);

  useEffect(() => {
    if (!open) return;
    const closeOnPointerDown = (event: PointerEvent) => {
      if (!root.current?.contains(event.target as Node)) setOpen(false);
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || event.isComposing || event.keyCode === 229) return;
      event.preventDefault();
      event.stopPropagation();
      setOpen(false);
      root.current?.querySelector<HTMLButtonElement>(".icon-action")?.focus();
    };
    document.addEventListener("pointerdown", closeOnPointerDown);
    document.addEventListener("keydown", closeOnEscape);
    return () => {
      document.removeEventListener("pointerdown", closeOnPointerDown);
      document.removeEventListener("keydown", closeOnEscape);
    };
  }, [open]);

  const create = async (request?: CreateSessionRequest) => {
    setError(undefined);
    try {
      await props.onCreate(request);
      setOpen(false);
    } catch (failure) {
      setError(failure);
    }
  };
  const choose = async () => {
    setError(undefined);
    try {
      const selection = await chooseDirectory();
      if (selection.type === "canceled") return;
      await create({ workspace: { path: selection.path } });
    } catch (failure) {
      setError(failure);
    }
  };
	const importArtifact = async () => {
		setError(undefined);
		try {
			const imported = await props.onImport();
			if (imported !== undefined) setOpen(false);
		} catch (failure) {
			setError(failure);
		}
	};

  return (
    <div className="new-session-menu window-no-drag" ref={root}>
      <button
        className="icon-action"
        type="button"
        aria-label={t("session.new")}
        title={t("session.newShortcut")}
        aria-keyshortcuts={ariaKeyShortcuts(
          commandByID("session.new").shortcut,
        )}
        aria-expanded={open}
        aria-controls={menuId}
        disabled={props.pending}
        onClick={() => {
          setError(undefined);
          setOpen((visible) => !visible);
        }}
      >
        <span aria-hidden="true">＋</span>
      </button>
        <section
          className={`new-session-popover t-dropdown${open ? " is-open" : closing ? " is-closing" : ""}`}
          data-origin="top-end"
          id={menuId}
          aria-label={t("session.new")}
          aria-hidden={!open}
          inert={!open}
        >
          <header>
            <strong>{t("session.start")}</strong>
            <span>{t("session.shortcut")}</span>
          </header>
          <button
            type="button"
            disabled={props.pending}
            onClick={() => void create()}
          >
            <span aria-hidden="true">↗</span>
            <span>
              <strong>{t("session.defaultWorkspace")}</strong>
              <small title={props.defaultWorkspace}>
                {compactPath(props.defaultWorkspace)}
              </small>
            </span>
          </button>
          <button
            type="button"
            disabled={props.pending}
            onClick={() => void choose()}
          >
            <span aria-hidden="true">⌁</span>
            <span>
              <strong>{t("session.chooseFolder")}</strong>
              <small>{t("session.chooseFolderDetail")}</small>
            </span>
          </button>
		  <button
			type="button"
			disabled={props.pending}
			onClick={() => void importArtifact()}
		  >
			<span aria-hidden="true">⇣</span>
			<span>
			  <strong>{t("session.import")}</strong>
			  <small>{t("session.importDetail")}</small>
			</span>
		  </button>
          {error ? (
            <p role="alert">{presentRuntimeError(error, t("session.createFailed"), t)}</p>
          ) : null}
        </section>
    </div>
  );
}

function useDropdownClosing(open: boolean): boolean {
  const wasOpen = useRef(open);
  const [closing, setClosing] = useState(false);

  useEffect(() => {
    let timer: number | undefined;
    if (open) {
      setClosing(false);
    } else if (wasOpen.current) {
      setClosing(true);
      const closeMilliseconds =
        Number.parseFloat(
          getComputedStyle(document.documentElement).getPropertyValue(
            "--dropdown-close-dur",
          ),
        ) || 150;
      timer = window.setTimeout(() => setClosing(false), closeMilliseconds);
    }
    wasOpen.current = open;
    return () => {
      if (timer !== undefined) window.clearTimeout(timer);
    };
  }, [open]);

  return closing;
}
