import type {HTMLAttributes, ReactNode} from 'react';

/** Small status or metadata label. */
export interface ChipProps extends HTMLAttributes<HTMLSpanElement> {
  /** Soft semantic fill. Omit for the neutral `--default` chip. */
  color?: 'accent' | 'success' | 'warning' | 'danger';
  /** @default 'md' */
  size?: 'sm' | 'md' | 'lg';
  /** Transparent with a hairline instead of a fill. */
  variant?: 'solid' | 'outline';
  /** Leading status dot. @default false */
  dot?: boolean;
  /** Renders a trailing × when provided. */
  onRemove?: () => void;
  children?: ReactNode;
}
export declare function Chip(props: ChipProps): JSX.Element;
