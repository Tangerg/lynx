import type {HTMLAttributes} from 'react';

/** Full colour picker panel. */
export interface ColorPickerProps extends Omit<HTMLAttributes<HTMLDivElement>, 'onChange'> {
  /** @default '#2E6FF2' */
  value?: string;
  /** Hue in degrees. @default 220 */
  hue?: number;
  onChange?: (value: string) => void;
  /** @default true */
  showAlpha?: boolean;
}
export declare function ColorPicker(props: ColorPickerProps): JSX.Element;
