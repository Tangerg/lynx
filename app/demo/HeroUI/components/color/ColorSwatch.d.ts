import type {HTMLAttributes} from 'react';

/** Preview chip for a single colour value. */
export interface ColorSwatchProps extends HTMLAttributes<HTMLSpanElement> {
  /** Any CSS colour, including `var(--accent)`. */
  color?: string;
  /** @default 'rounded' */
  shape?: 'rounded' | 'circle';
  /** Square size in px. @default 32 */
  size?: number;
}
export declare function ColorSwatch(props: ColorSwatchProps): JSX.Element;
