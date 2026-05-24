import { Icon } from "@/components/common";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";
import { definePlugin } from "@/plugins/sdk";

// Placeholder input until a global "search files / commands" lands. A
// future plugin replaces this with a real local-files index.

function SidebarSearch() {
  const t = useT();
  return (
    <div className="relative mx-1 mb-3.5">
      <div className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-fg-faint">
        <Icon name="search" size={14} />
      </div>
      <input
        type="search"
        aria-label={t("sidebar.search.label")}
        placeholder={t("sidebar.search.placeholder")}
        // Dark: shadow-input/focus tokens give a soft inset glow.
        // Light: tokens go quiet (Vercel pattern) so we draw shape with
        // a hairline border + accent focus ring instead.
        className={cn(
          "w-full rounded-sm border-0 bg-surface-2 py-2 pl-9 pr-3 font-sans text-[13px] text-fg outline-none placeholder:text-fg-faint",
          "shadow-[var(--shadow-input)] focus:shadow-[var(--shadow-input-focus)]",
          "light:border light:border-line light:shadow-none",
          "light:focus:border-accent light:focus:shadow-[0_0_0_3px_color-mix(in_srgb,var(--color-accent)_12%,transparent)]",
        )}
      />
      <span className="absolute right-2.5 top-1/2 -translate-y-1/2 rounded-[4px] bg-surface-2 light:bg-surface-3 px-1.5 py-px font-mono text-[11px] text-fg-faint tracking-normal">
        ⌘K
      </span>
    </div>
  );
}

export const sidebarSearch = definePlugin({
  name: "lyra.builtin.sidebar-search",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("sidebar.search", { id: "default", order: 0, component: SidebarSearch });
  },
});
