import type {HTMLAttributes, ReactNode} from 'react';

/** Keycap for shortcut hints. */
export interface KbdProps extends HTMLAttributes<HTMLElement> {
  /** Modifier ids mapped to unicode glyphs: cmd, shift, alt, ctrl, enter, esc, tab, up… */
  keys?: string | string[];
  children?: ReactNode;
}
export declare function Kbd(props: KbdProps): JSX.Element;
