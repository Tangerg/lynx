import { useRef } from "react";

import { Button, DropdownMenu, HiddenFileInput, Icon, IconButton, ProviderIcon } from "@/ui";
import { imageFiles } from "@/plugins/builtin/chat/composer/public/input";
import { useSelectedModel } from "./public/selectedModel";
import {
  APPROVAL_MODES,
  DEFAULT_APPROVAL_MODE,
  setApprovalMode,
  useApprovalMode,
  type ApprovalMode,
} from "@/plugins/builtin/agent/public/approvalPolicy";
import { useModels } from "@/plugins/builtin/settings/providers/public/queries";
import { rpcErrorText } from "@/lib/rpcErrors";
import { contributeLayout, notifyError } from "@/plugins/sdk";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/classNames";
import { definePlugin } from "@/plugins/sdk";
import { useAddComposerImageFiles } from "./public/attachments";
import { useSetComposerModelPreference } from "./public/modelPreference";

// The trigger wears the selected model's provider mark. Provider health is not
// part of this app's state, so the control carries no status indicator.
function ModelPicker() {
  const t = useT();
  const { data: models = [], isLoading, isError } = useModels();
  const setModel = useSetComposerModelPreference();
  const selected = useSelectedModel();

  if (models.length === 0) {
    if (isError) {
      return (
        <Button
          variant="ghost"
          disabled
          title={t("providers.models.error")}
          className="gap-1.5 px-2 text-ui-sm text-negative"
        >
          <Icon name="alert" size="sm" />
          <span>{t("providers.models.error")}</span>
        </Button>
      );
    }
    if (!isLoading) return null;
    return (
      <div
        className="inline-flex h-[var(--control-height-md)] shrink-0 items-center gap-1.5 rounded-md px-2.5 opacity-60"
        aria-hidden
      >
        <span className="h-1.5 w-1.5 rounded-full bg-surface-2" />
        <span className="h-3 w-16 rounded-sm bg-surface-2" />
      </div>
    );
  }
  // An active Session id can be available one query tick before its summary.
  // Keep the placeholder for that tick instead of showing a catalog model
  // which would disagree with the Session's durable model.
  if (!selected) {
    return (
      <div
        className="inline-flex h-[var(--control-height-md)] shrink-0 items-center gap-1.5 rounded-md px-2.5 opacity-60"
        aria-hidden
      >
        <span className="h-1.5 w-1.5 rounded-full bg-surface-2" />
        <span className="h-3 w-16 rounded-sm bg-surface-2" />
      </div>
    );
  }

  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger
        render={
          <Button
            variant="ghost"
            aria-label={t("composer.switchModel")}
            className="gap-1.5 whitespace-nowrap px-2 text-ui-sm text-fg-soft data-[popup-open]:bg-selected data-[popup-open]:text-fg"
          >
            <ProviderIcon provider={selected.provider} size="sm" />
            <span className="max-w-[168px] truncate">{selected.label}</span>
            <Icon name="chevron-down" size="sm" className="shrink-0 text-fg-faint" />
          </Button>
        }
      />
      <DropdownMenu.Content align="start" sideOffset={6} className="min-w-[200px]">
        {models.map((m) => (
          <DropdownMenu.Item
            key={`${m.provider}:${m.id}`}
            onClick={() => setModel(m.provider, m.id)}
            className="grid-cols-[16px_minmax(0,1fr)_14px] px-2"
          >
            <ProviderIcon provider={m.provider} size="md" />
            <span className="truncate">{m.label}</span>
            {m.provider === selected.provider && m.id === selected.id && (
              <Icon name="check" size="xs" className="text-accent" />
            )}
          </DropdownMenu.Item>
        ))}
      </DropdownMenu.Content>
    </DropdownMenu.Root>
  );
}

function AttachButton() {
  const t = useT();
  const addImageFiles = useAddComposerImageFiles();
  const inputRef = useRef<HTMLInputElement>(null);
  const canAttach = useSelectedModel()?.multimodal ?? false;

  return (
    <>
      <HiddenFileInput
        ref={inputRef}
        accept="image/*"
        multiple
        aria-label={t("composer.attachImage")}
        onChange={(e) => {
          const files = imageFiles(e.target.files);
          e.target.value = "";
          if (files.length > 0) addImageFiles(files);
        }}
      />
      <IconButton
        icon="plus"
        aria-label={t("composer.attachImage")}
        title={canAttach ? t("composer.attachImage") : t("composer.attachImage.unsupported")}
        disabled={!canAttach}
        onClick={() => inputRef.current?.click()}
        className="disabled:opacity-25"
      />
    </>
  );
}

// Approval-mode pill — the composer's primary access control.
// A ghost pill that turns warning-toned when full access ("yolo") is on.
function ApprovalModePill() {
  const t = useT();
  const { data: mode, isError } = useApprovalMode();
  if (isError || mode === undefined) return null;
  const current = APPROVAL_MODES.find((m) => m.value === mode) ?? DEFAULT_APPROVAL_MODE;
  const full = mode === "yolo";
  const onSelect = async (next: ApprovalMode) => {
    if (next === mode) return;
    try {
      await setApprovalMode(next);
    } catch (err) {
      notifyError(rpcErrorText(err) ?? t("approvals.error.mode"));
    }
  };
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger
        render={
          <Button
            type="button"
            variant="ghost"
            size="md"
            press={false}
            aria-label={t("approvals.mode.aria")}
            className={cn(
              "inline-flex shrink-0 items-center gap-1.5 whitespace-nowrap rounded-[var(--button-radius)] border-0 px-2 font-sans text-ui-sm font-medium transition-colors data-[popup-open]:bg-selected",
              full
                ? "text-warning hover:bg-warning-wash"
                : "text-fg-soft hover:bg-hover hover:text-fg",
            )}
          >
            <Icon name={full ? "alert" : "shield"} size="sm" className="shrink-0 opacity-100" />
            <span className="max-w-[132px] truncate">{t(current.labelKey)}</span>
            <Icon name="chevron-down" size="sm" className="shrink-0 text-fg-faint opacity-100" />
          </Button>
        }
      />
      <DropdownMenu.Content align="start" sideOffset={6} className="min-w-[248px]">
        {APPROVAL_MODES.map((m) => (
          <DropdownMenu.Item
            key={m.value}
            onClick={() => void onSelect(m.value)}
            className="grid grid-cols-[minmax(0,1fr)_14px] items-start gap-2 rounded-md px-2 py-1.5 outline-none data-[highlighted]:bg-hover"
          >
            <span className="min-w-0">
              <span className="block text-ui-md font-semibold text-fg">{t(m.labelKey)}</span>
              <span className="block text-ui-sm leading-snug text-fg-muted">{t(m.descKey)}</span>
            </span>
            {m.value === mode && <Icon name="check" size="xs" className="mt-0.5 text-accent" />}
          </DropdownMenu.Item>
        ))}
      </DropdownMenu.Content>
    </DropdownMenu.Root>
  );
}

export const composerToolbar = definePlugin({
  name: "lyra.builtin.composer-toolbar",
  setup(ctx) {
    contributeLayout(ctx, "composer.toolbar.start", {
      id: "attach",
      order: 0,
      component: AttachButton,
    });
    contributeLayout(ctx, "composer.toolbar.start", {
      id: "approval",
      order: 1,
      component: ApprovalModePill,
    });
    contributeLayout(ctx, "composer.toolbar.start", {
      id: "model",
      order: 2,
      component: ModelPicker,
    });
  },
});
