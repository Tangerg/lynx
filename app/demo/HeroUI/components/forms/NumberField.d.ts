import type {InputHTMLAttributes} from 'react';

/** Numeric input with increment/decrement steppers. */
export interface NumberFieldProps extends Omit<InputHTMLAttributes<HTMLInputElement>, 'size' | 'value' | 'defaultValue' | 'onChange' | 'step' | 'min' | 'max'> {
  value?: number;
  /** @default 0 */
  defaultValue?: number;
  /** @default 1 */
  step?: number;
  min?: number;
  max?: number;
  onChange?: (value: number) => void;
}
export declare function NumberField(props: NumberFieldProps): JSX.Element;
