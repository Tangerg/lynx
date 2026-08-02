import type {HTMLAttributes, ReactNode} from 'react';

/** Inline message tied to the surrounding content. */
export interface AlertProps extends HTMLAttributes<HTMLDivElement> {
  /** Soft semantic fill. Omit for neutral. */
  color?: 'accent' | 'success' | 'warning' | 'danger';
  title?: ReactNode;
  /** Override the default glyph; pass `null` to drop it. */
  icon?: ReactNode;
  /** Renders a dismiss button when provided. */
  onClose?: () => void;
  children?: ReactNode;
}
export declare function Alert(props: AlertProps): JSX.Element;
