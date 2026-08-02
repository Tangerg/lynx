import type {HTMLAttributes, ReactNode} from 'react';

/** Boolean choice with an optional description line. */
export interface CheckboxProps extends Omit<HTMLAttributes<HTMLLabelElement>, 'onChange'> {
  isSelected?: boolean;
  /** @default false */
  defaultSelected?: boolean;
  /** @default false */
  isIndeterminate?: boolean;
  /** @default false */
  isInvalid?: boolean;
  /** @default false */
  isDisabled?: boolean;
  description?: ReactNode;
  onChange?: (isSelected: boolean) => void;
  children?: ReactNode;
}
export declare function Checkbox(props: CheckboxProps): JSX.Element;
