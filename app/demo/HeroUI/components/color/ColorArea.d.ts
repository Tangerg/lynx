import type {HTMLAttributes} from 'react';

/** Two-dimensional saturation/brightness picker. */
export interface ColorAreaProps extends HTMLAttributes<HTMLDivElement> {
  /** Hue in degrees. @default 220 */
  hue?: number;
  /** Thumb position, 0–1. */
  x?: number;
  y?: number;
}
export declare function ColorArea(props: ColorAreaProps): JSX.Element;
