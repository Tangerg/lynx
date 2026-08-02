import type {HTMLAttributes} from 'react';

/** Choose one colour from a curated palette. */
export interface ColorSwatchPickerProps extends Omit<HTMLAttributes<HTMLDivElement>, 'onChange'> {
  colors?: string[];
  value?: string;
  onChange?: (color: string) => void;
  /** @default 32 */
  size?: number;
}
export declare function ColorSwatchPicker(props: ColorSwatchPickerProps): JSX.Element;
