import type {HTMLAttributes, ReactNode} from 'react';

/** Brief label revealed on hover or focus. */
export interface TooltipProps extends HTMLAttributes<HTMLSpanElement> {
  content?: ReactNode;
  /** Optional trailing keycap. */
  shortcut?: ReactNode;
  /** @default 'top' */
  placement?: 'top' | 'bottom';
  children?: ReactNode;
}
export declare function Tooltip(props: TooltipProps): JSX.Element;
