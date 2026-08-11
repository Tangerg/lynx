// Traces tab — the span list. Each row is a click target that expands a detail
// panel (status.message + ids / timing / attributes), so an error reads as more
// than a red tag.

import type { ReactNode } from "react";
import type { SpanRow } from "@/lib/observability/stores";
import { useTelemetryStore } from "@/lib/observability/stores";
import { Fragment, useCallback, useId, useMemo, useState } from "react";
import { useT } from "@/lib/i18n";
import { Icon, Pressable } from "@/ui";
import { cn } from "@/lib/classNames";
import { Cell, Empty, Row, VirtualList } from "./primitives";

export function TracesPanel() {
  const t = useT();
  const spans = useTelemetryStore((s) => s.spans);
  // Newest first. spans changes only once per flush (~500ms) so the reverse
  // copy is cheap and memoized on the (stable-between-flushes) array ref.
  const ordered = useMemo(() => spans.slice().reverse(), [spans]);
  // Expanded span ids — keyed by stable spanId so it survives the next flush
  // (incoming spans don't disturb which rows the user opened).
  const [expanded, setExpanded] = useState<ReadonlySet<string>>(() => new Set());
  const toggle = useCallback((id: string) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  if (ordered.length === 0) return <Empty hint={t("diagnostics.empty.traces")} />;

  return (
    <VirtualList
      count={ordered.length}
      rowHeight={32}
      header={
        <Row head>
          <Cell className="w-4" />
          <Cell className="grow">span</Cell>
          <Cell className="w-16 text-right">dur</Cell>
          <Cell className="w-16">status</Cell>
          <Cell className="w-28">trace</Cell>
        </Row>
      }
      renderRow={(i) => {
        const s = ordered[i]!;
        return <SpanRowItem span={s} open={expanded.has(s.id)} onToggle={() => toggle(s.id)} />;
      }}
    />
  );
}

const STATUS_TONE: Record<SpanRow["status"], string> = {
  error: "text-negative",
  ok: "text-success",
  unset: "text-fg-faint",
};

function StatusTag({ status }: { status: SpanRow["status"] }) {
  return <span className={STATUS_TONE[status]}>{status}</span>;
}

// One trace row: a click target that toggles a detail panel below it. The
// detail is where the "why" lives — status.message (the failure text) plus the
// span's ids / timing / attributes — so an error reads as more than a red tag.
function SpanRowItem({
  span,
  open,
  onToggle,
}: {
  span: SpanRow;
  open: boolean;
  onToggle: () => void;
}) {
  const panelId = useId();
  return (
    <div>
      <Pressable
        type="button"
        onClick={onToggle}
        aria-expanded={open}
        aria-controls={panelId}
        className="flex min-h-8 w-full items-center gap-3 bg-transparent px-1 font-mono text-ui-md text-fg hover:bg-hover"
      >
        <span className="flex w-4 shrink-0 justify-center">
          <Icon
            name="chevron-down"
            size="xs"
            className={cn("text-fg-faint transition-transform", !open && "-rotate-90")}
          />
        </span>
        <span className="grow min-w-0 truncate text-left">{span.name}</span>
        <span className="w-16 shrink-0 text-right tabular-nums">
          {span.durationMillis.toFixed(1)}ms
        </span>
        <span className="w-16 shrink-0 text-left">
          <StatusTag status={span.status} />
        </span>
        <span className="w-28 shrink-0 truncate text-left text-fg-faint">
          {span.traceId.slice(0, 12)}
        </span>
      </Pressable>
      {/* Wired to its panel, but deliberately NOT animated through `Collapsible`
          like the transcript's disclosures: these rows live in a VirtualList, and a
          220ms height transition is 220ms of the virtualiser re-measuring a row
          that is still moving. */}
      {open && (
        <div id={panelId}>
          <SpanDetail span={span} />
        </div>
      )}
    </div>
  );
}

// Expanded span detail — mono key/value block. Error message first (the thing
// the collapsed row can't carry), then ids/timing, then any attributes.
//
// The keys stay English and untranslated, all of them: `trace` / `span` /
// `parent` / `kind` / `start` / `dur` / `error` / `attributes` are OTel's own
// vocabulary, and this block is a data dump a developer matches against the spec,
// not prose. The panel's prose — its empty states — goes through the catalogs.
function SpanDetail({ span }: { span: SpanRow }) {
  const meta: [string, string][] = [
    ["trace", span.traceId],
    ["span", span.id],
    ["parent", span.parentSpanId ?? "—"],
    ["kind", span.kind],
    ["start", new Date(span.startMs).toISOString()],
    ["dur", `${span.durationMillis.toFixed(1)}ms`],
  ];
  const attrs = Object.entries(span.attrs);
  return (
    <div className="mx-1 mb-1.5 grid gap-2 rounded-md bg-sunken px-3 py-2 font-mono text-ui-sm">
      {span.statusMessage && (
        <Field label="error">
          <span className="whitespace-pre-wrap break-words text-negative select-text">
            {span.statusMessage}
          </span>
        </Field>
      )}
      <KeyValues rows={meta} />
      {attrs.length > 0 && (
        <Field label="attributes">
          <KeyValues rows={attrs.map(([k, v]) => [k, String(v)])} />
        </Field>
      )}
    </div>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid gap-0.5">
      <div className="text-ui-xs text-fg-faint">{label}</div>
      {children}
    </div>
  );
}

function KeyValues({ rows }: { rows: [string, string][] }) {
  return (
    <div className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-0.5">
      {rows.map(([k, v]) => (
        <Fragment key={k}>
          <div className="text-fg-faint">{k}</div>
          <div className="break-all text-fg-muted select-text">{v}</div>
        </Fragment>
      ))}
    </div>
  );
}
