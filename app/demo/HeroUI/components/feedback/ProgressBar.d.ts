import type {HTMLAttributes, ReactNode} from 'react';

/** Horizontal progress track. (v2's `Progress`.) */
export interface ProgressBarProps extends HTMLAttributes<HTMLDivElement> {
  /** @default 0 */
  value?: number;
  /** @default 100 */
  max?: number;
  label?: ReactNode;
  /** @default false */
  showValue?: boolean;
  color?: 'success' | 'danger';
  /** @default false */
  isIndeterminate?: boolean;
}
export declare function ProgressBar(props: ProgressBarProps): JSX.Element;
