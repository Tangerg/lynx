// Composer for the appearance settings pane. The component itself only lays out
// sections; each section owns its own preference subscription.

import { AgentSurface } from "@/components/agent-studio";
import { useT } from "@/lib/i18n";
import { AccentSection } from "./AccentSection";
import { ContrastSection } from "./ContrastSection";
import { CustomThemeColors } from "./CustomThemeColors";
import { FontSection } from "./FontSection";
import { LanguageSection } from "./LanguageSection";
import { ShapeMotionSection } from "./ShapeMotionSection";
import { ThemeSection } from "./ThemeSection";

function ThemePreviewStrip() {
  const t = useT();
  return (
    <div className="mt-8 grid grid-cols-3 gap-4">
      <button
        type="button"
        className="group flex flex-col items-center gap-3 border-0 bg-transparent p-0 text-center"
      >
        <div className="h-[86px] w-full overflow-hidden rounded-[14px] bg-[#111] shadow-[inset_0_0_0_0.5px_rgb(255_255_255/0.16)]">
          <div className="h-full w-[38%] bg-[#e8e8e9]" />
        </div>
        <span className="text-[13px] font-medium text-fg-soft group-hover:text-fg">
          {t("settings.theme.system")}
        </span>
      </button>
      <button
        type="button"
        className="group flex flex-col items-center gap-3 border-0 bg-transparent p-0 text-center"
      >
        <div className="h-[86px] w-full overflow-hidden rounded-[14px] bg-canvas shadow-[0_0_0_2px_var(--color-accent),inset_0_0_0_0.5px_var(--color-field)]">
          <div className="h-full w-[38%] bg-surface" />
        </div>
        <span className="text-[13px] font-semibold text-fg">{t("settings.theme.light")}</span>
      </button>
      <button
        type="button"
        className="group flex flex-col items-center gap-3 border-0 bg-transparent p-0 text-center"
      >
        <div className="h-[86px] w-full overflow-hidden rounded-[14px] bg-[#17181c] shadow-[inset_0_0_0_0.5px_rgb(255_255_255/0.14)]">
          <div className="h-full w-[38%] bg-[#2b2c31]" />
        </div>
        <span className="text-[13px] font-medium text-fg-soft group-hover:text-fg">
          {t("settings.theme.dark")}
        </span>
      </button>
    </div>
  );
}

function ThemeCodePreview() {
  return (
    <AgentSurface className="mt-8 grid grid-cols-2 overflow-hidden bg-surface">
      <div className="border-r-[0.5px] border-field/70 p-5 font-mono text-[12.5px] leading-7 text-fg-muted">
        <div>
          <span className="text-[#c084fc]">const</span> theme = {"{"}
        </div>
        <div className="mt-2 rounded-[7px] bg-success/10 px-3 py-1 text-fg">
          surface: <span className="text-success">"sidebar"</span>,
          <br />
          accent: <span className="text-success">"#d92662"</span>,
        </div>
        <div className="mt-2">contrast: 42</div>
        <div>{"};"}</div>
      </div>
      <div className="p-5 font-mono text-[12.5px] leading-7 text-fg-muted">
        <div>
          <span className="text-[#c084fc]">const</span> theme = {"{"}
        </div>
        <div className="mt-2 rounded-[7px] bg-negative/10 px-3 py-1 text-fg">
          surface: <span className="text-negative">"sidebar-elevated"</span>,
          <br />
          accent: <span className="text-negative">"#0ea5e9"</span>,
        </div>
        <div className="mt-2">contrast: 68</div>
        <div>{"};"}</div>
      </div>
    </AgentSurface>
  );
}

export function AppearancePane() {
  const t = useT();
  return (
    <div className="pb-16">
      <header>
        <h1 className="m-0 text-[28px] font-semibold text-fg">{t("settings.pane.appearance")}</h1>
        <p className="m-0 mt-2 text-[13.5px] leading-6 text-fg-muted">
          {t("settings.appearance.hero")}
        </p>
      </header>
      <ThemePreviewStrip />
      <ThemeCodePreview />
      <AgentSurface className="mt-10 divide-y divide-field/70 overflow-hidden bg-canvas">
        <ThemeSection />
        <CustomThemeColors />
        <AccentSection />
        <ContrastSection />
        <FontSection />
        <ShapeMotionSection />
        <LanguageSection />
      </AgentSurface>
    </div>
  );
}
