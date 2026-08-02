import type {HTMLAttributes, ReactNode} from 'react';

/** Immediate on/off control — settings, not forms you submit. */
export interface SwitchProps extends Omit<HTMLAttributes<HTMLLabelElement>, 'onChange'> {
  isSelected?: boolean;
  /** @default false */
  defaultSelected?: boolean;
  /** @default false */
  isDisabled?: boolean;
  onChange?: (isSelected: boolean) => void;
  /** @default 'end' */
  labelPosition?: 'start' | 'end';
  children?: ReactNode;
}
export declare function Switch(props: SwitchProps): JSX.Element;
