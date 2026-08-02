import type {HTMLAttributes} from 'react';

/** Circular progress. (v2's `CircularProgress`.) */
export interface ProgressCircleProps extends HTMLAttributes<HTMLSpanElement> {
  /** @default 0 */
  value?: number;
  /** @default 100 */
  max?: number;
  /** Diameter in px. @default 48 */
  size?: number;
  /** Stroke width in px. @default 4 */
  thickness?: number;
  /** @default false */
  showValue?: boolean;
}
export declare function ProgressCircle(props: ProgressCircleProps): JSX.Element;
