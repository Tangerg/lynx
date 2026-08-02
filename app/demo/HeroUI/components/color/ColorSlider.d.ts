import type {HTMLAttributes} from 'react';

/** Single-channel colour track. */
export interface ColorSliderProps extends HTMLAttributes<HTMLDivElement> {
  /** @default 'hue' */
  channel?: 'hue' | 'alpha';
  /** Thumb position, 0–1. @default 0.5 */
  value?: number;
  /** Fill colour for the alpha ramp and the thumb. */
  color?: string;
}
export declare function ColorSlider(props: ColorSliderProps): JSX.Element;
