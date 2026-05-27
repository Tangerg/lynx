import type { Attachment, ComposerMode } from "@/state/composerStore";
import type { IconName } from "@/components/common";
import * as DropdownMenu from "@radix-ui/react-dropdown-menu";
import { useEffect, useMemo, useRef } from "react";
import { Chip, Icon, Tooltip } from "@/components/common";
import { useT } from "@/lib/i18n";
import {
  lookupComposerKeyBinding,
  normalizeCombo,
  pickComposerPlaceholder,
  useComposerAttachmentSources,
  useComposerModes,
} from "@/plugins/sdk";
import { Slot } from "@/plugins/Slot";
import { submitComposer } from "./submitComposer";

interface Props {
  onSend: (text: string) => void;
  value: string;
  onChange: (v: string) => void;
  attachments: Attachment[];
  onRemoveAttachment: (i: number) => void;
  mode: ComposerMode;
  onModeChange: (m: ComposerMode) => void;
}

export function Composer({
  onSend,
  value,
  onChange,
  attachments,
  onRemoveAttachment,
  mode,
  onModeChange,
}: Props) {
  const t = useT();
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const modes = useComposerModes();
  const attachmentSources = useComposerAttachmentSources();
  // Pick a placeholder once at mount — `pickComposerPlaceholder` is
  // random, so re-running on every render would make the text flicker.
  // Falls back to the localized "ask…" string if no placeholder plugin
  // is registered (or the random pick rolls a 0-weight pool). Locale
  // switch after mount won't relocalize the fallback — acceptable for
  // a random hint string.
  const placeholder = useMemo(
    () => pickComposerPlaceholder()?.text ?? t("composer.placeholder.fallback"),
    // eslint-disable-next-line react/exhaustive-deps
    [],
  );

  const submit = () =>
    submitComposer({
      value,
      clear: () => onChange(""),
      sendText: onSend,
    });

  useEffect(() => {
    const el = inputRef.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, 160)}px`;
  }, [value]);

  return (
    <div className="relative rounded-2xl border border-line-soft bg-surface px-2.5 pb-1.5 pt-2 transition-[border-color,box-shadow] duration-150 focus-within:border-line-soft focus-within:shadow-[0_0_0_3px_color-mix(in_srgb,var(--color-accent)_14%,transparent)]">
      <PluginAttachments sources={attachmentSources} />
      {attachments.length > 0 && (
        <div className="flex flex-wrap gap-1.5 px-1 pb-0.5 pt-1">
          {attachments.map((a, i) => (
            <Chip
              key={a.id}
              icon={(a.icon as IconName | undefined) ?? "file"}
              title={a.label}
              onClose={() => onRemoveAttachment(i)}
            >
              {a.label}
            </Chip>
          ))}
        </div>
      )}
      <textarea
        ref={inputRef}
        aria-label={t("composer.input.label")}
        placeholder={placeholder}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onKeyDown={(e) => {
          // Resolve the pressed combo to its canonical form and ask the
          // plugin registry for a binding. The built-in "composer-keymap"
          // plugin registers Enter→submit; user plugins can add more
          // (e.g. Mod+K to open a snippet drawer).
          const parts: string[] = [];
          if (e.metaKey || e.ctrlKey) parts.push("mod");
          if (e.altKey) parts.push("alt");
          if (e.shiftKey) parts.push("shift");
          parts.push(e.key);
          const binding = lookupComposerKeyBinding(normalizeCombo(parts.join("+")));
          if (!binding) return;
          const handled = binding.handler({
            value,
            onChange,
            submit,
            event: e.nativeEvent,
          });
          if (handled) e.preventDefault();
        }}
        rows={1}
        /* The `composer-input` class is a DOM-target hook (no styles) so
           the `composer.focus` command in defaults/commands.ts can find
           this textarea without threading a ref through the tree. */
        className="composer-input w-full resize-none border-0 bg-transparent px-1.5 py-2 font-sans text-[15px] leading-[1.55] tracking-[-0.003em] text-fg outline-none min-h-5.5 max-h-40 placeholder:text-fg-faint placeholder:tracking-normal"
      />
      <div className="flex flex-nowrap items-center gap-1 pt-1.5">
        <Slot name="composer.toolbar.start" />
        {modes.length > 0 && <ModePicker modes={modes} value={mode} onChange={onModeChange} />}
        <div className="flex-1 min-w-2" />
        <Slot name="composer.toolbar.end" />
      </div>
    </div>
  );
}

// Compact mode picker — Radix DropdownMenu trigger that shows the
// current mode (icon + label + chevron) and reveals the full list on
// click. Replaces the wider Segmented control to give the composer
// toolbar more breathing room.
type Mode = ReturnType<typeof useComposerModes>[number];

function ModePicker({
  modes,
  value,
  onChange,
}: {
  modes: Mode[];
  value: ComposerMode;
  onChange: (v: ComposerMode) => void;
}) {
  const active = modes.find((m) => m.id === value) ?? modes[0];
  return (
    <DropdownMenu.Root>
      <Tooltip label="Composer mode">
        <DropdownMenu.Trigger className="inline-flex h-6.5 items-center gap-1.5 rounded-sm border border-line bg-transparent px-2 font-sans text-[11.5px] font-semibold text-fg cursor-pointer transition-colors hover:bg-surface-2 data-[state=open]:bg-surface-2 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-accent">
          <Icon name={(active.icon as IconName) ?? "spark"} size={12} />
          <span>{active.label}</span>
          <Icon name="more" size={10} className="text-fg-faint -rotate-90" />
        </DropdownMenu.Trigger>
      </Tooltip>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          align="start"
          sideOffset={6}
          className="z-50 min-w-[240px] overflow-hidden rounded-md border border-line-soft bg-surface p-1 shadow-lg animate-rise-in"
        >
          {modes.map((m) => (
            <DropdownMenu.Item
              key={m.id}
              onSelect={() => onChange(m.id)}
              className="grid grid-cols-[16px_minmax(0,1fr)_12px] cursor-pointer items-start gap-2 rounded-sm px-2.5 py-1.5 text-[12.5px] text-fg-muted outline-none data-[highlighted]:bg-surface-2 data-[highlighted]:text-fg"
            >
              <Icon name={(m.icon as IconName) ?? "spark"} size={12} className="mt-0.5" />
              <div className="min-w-0">
                <div className="text-fg">{m.label}</div>
                {m.description && (
                  <div className="mt-0.5 text-[11px] leading-snug text-fg-faint">
                    {m.description}
                  </div>
                )}
              </div>
              {m.id === value ? (
                <Icon name="check" size={12} className="mt-0.5 text-accent" />
              ) : (
                <span aria-hidden />
              )}
            </DropdownMenu.Item>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}

// Render every plugin-contributed attachment source's `useAttachments()`
// output in one row. Each source's hook runs inside its own component so a
// throwing/buggy hook is isolated to that one bubble.
type AttachmentSource = ReturnType<typeof useComposerAttachmentSources>[number];
function PluginAttachments({ sources }: { sources: AttachmentSource[] }) {
  if (sources.length === 0) return null;
  return (
    <div className="flex flex-wrap gap-1.5 px-1 pb-0.5 pt-1">
      {sources.map((s) => (
        <SourceChips key={s.id} source={s} />
      ))}
    </div>
  );
}
function SourceChips({ source }: { source: AttachmentSource }) {
  const items = source.useAttachments();
  return (
    <>
      {items.map((a) => (
        <Chip
          key={`${source.id}:${a.id ?? a.label}`}
          icon={(a.icon as IconName | undefined) ?? "file"}
          title={a.label}
        >
          {a.label}
        </Chip>
      ))}
    </>
  );
}
