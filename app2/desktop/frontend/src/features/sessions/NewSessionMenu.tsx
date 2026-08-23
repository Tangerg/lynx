import { useEffect, useId, useRef, useState } from "react";

import type { CreateSessionRequest, Session } from "@lyra/runtime-contract";

import { useLocalization } from "../localization/Localization";
import { presentRuntimeError } from "../localization/presentRuntimeError";
import { chooseDirectory } from "../../runtime/desktopBridge";
import { compactPath } from "./sessionPresentation";
import {
  ariaKeyShortcuts,
  commandByID,
  shortcutTokens,
} from "../shell/commandCatalog";
import { Tooltip } from "../shell/Tooltip";
import { useActionMenu } from "../shell/useActionMenu";

interface NewSessionMenuProps {
  pending: boolean;
  defaultWorkspace: string;
  onCreate: (request?: CreateSessionRequest) => Promise<Session>;
	onImport: () => Promise<Session | undefined>;
}

export function NewSessionMenu(props: NewSessionMenuProps) {
  const { t } = useLocalization();
  const menuId = useId();
  const menu = useActionMenu<HTMLDivElement, HTMLButtonElement, HTMLElement>();
  const { open } = menu;
  const [error, setError] = useState<unknown>();
  const closing = useDropdownClosing(open);

  const create = async (request?: CreateSessionRequest) => {
    setError(undefined);
    try {
      await props.onCreate(request);
      menu.close();
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
			if (imported !== undefined) menu.close();
		} catch (failure) {
			setError(failure);
		}
	};

  return (
    <div className="new-session-menu window-no-drag" ref={menu.rootRef}>
      <Tooltip
        label={t("session.new")}
        shortcut={shortcutTokens(commandByID("session.new").shortcut)}
      >
        <button
          ref={menu.triggerRef}
          className="icon-action"
          type="button"
          aria-label={t("session.new")}
          aria-keyshortcuts={ariaKeyShortcuts(
            commandByID("session.new").shortcut,
          )}
          aria-haspopup="menu"
          aria-expanded={open}
          aria-controls={menuId}
          disabled={props.pending}
          onClick={() => {
            if (!open) setError(undefined);
            menu.toggle();
          }}
        >
          <span aria-hidden="true">＋</span>
        </button>
      </Tooltip>
        <section
          ref={menu.menuRef}
          className={`new-session-popover t-dropdown${open ? " is-open" : closing ? " is-closing" : ""}`}
          data-origin="top-end"
          id={menuId}
          role="menu"
          aria-label={t("session.new")}
          aria-hidden={!open}
          inert={!open}
        >
          <header role="presentation">
            <strong>{t("session.start")}</strong>
            <span>{t("session.shortcut")}</span>
          </header>
          <button
            type="button"
            role="menuitem"
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
            role="menuitem"
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
			role="menuitem"
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
