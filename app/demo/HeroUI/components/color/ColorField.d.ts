import type {HTMLAttributes} from 'react';

/** Monospace hex input with an inline preview swatch. */
export interface ColorFieldProps extends Omit<HTMLAttributes<HTMLDivElement>, 'onChange'> {
  /** @default '#2E6FF2' */
  value?: string;
  onChange?: (value: string) => void;
}
export declare function ColorField(props: ColorFieldProps): JSX.Element;
