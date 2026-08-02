import type {HTMLAttributes, ReactNode} from 'react';

/** A measurement inside a known range — not task progress. */
export interface MeterProps extends HTMLAttributes<HTMLDivElement> {
  /** @default 0 */
  value?: number;
  /** @default 100 */
  max?: number;
  label?: ReactNode;
  /** @default true */
  showValue?: boolean;
  color?: 'warning' | 'danger';
}
export declare function Meter(props: MeterProps): JSX.Element;
