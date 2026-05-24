import { Icon } from "@/components/common";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";

// `.brand-mark` is kept as a hook for the Wails drag-region opt-out
// defined in overlays.css. All visual styling lives in Tailwind utilities.

function BrandMark({ extra }: { extra?: string }) {
  return (
    <div
      className={cn(
        // Dark accent (#1ed760) reads black ink; light accent (#15883e) needs white.
        "brand-mark grid h-7 w-7 place-items-center rounded-full bg-accent text-black light:text-white",
        extra,
      )}
    >
      <Icon name="spark" size={16} />
    </div>
  );
}

function FullBrand() {
  return (
    <>
      <BrandMark />
      <div>
        <div className="font-sans text-[17px] font-extrabold tracking-[-0.01em]">Lyra</div>
      </div>
    </>
  );
}

// In rail mode the wrapper itself is the 36×36 accent square; brand-mark
// just hosts the icon (no extra background) and uses `contents` so it
// doesn't introduce a second visual box.
function RailBrand() {
  return (
    <div className="brand-mark contents">
      <Icon name="spark" size={16} />
    </div>
  );
}

export const sidebarBrand = definePlugin({
  name: "lyra.builtin.sidebar-brand",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("sidebar.brand", { id: "brand", order: 0, component: FullBrand });
    host.layout.register("sidebar.rail.brand", {
      id: "rail-brand",
      order: 0,
      component: RailBrand,
    });
  },
});
