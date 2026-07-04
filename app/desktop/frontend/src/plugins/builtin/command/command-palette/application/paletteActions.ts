import { usePaletteStore } from "../../paletteStore";

export function openCommandPalette() {
  usePaletteStore.getState().setOpen(true);
}

export function toggleCommandPalette() {
  usePaletteStore.getState().toggle();
}
