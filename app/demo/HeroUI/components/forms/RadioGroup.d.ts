import type {HTMLAttributes, ReactNode} from 'react';

/** Mutually exclusive choice. */
export interface RadioGroupProps extends Omit<HTMLAttributes<HTMLDivElement>, 'onChange' | 'defaultValue'> {
  label?: ReactNode;
  value?: string;
  defaultValue?: string;
  onChange?: (value: string) => void;
  /** @default 'vertical' */
  orientation?: 'vertical' | 'horizontal';
  children?: ReactNode;
}
export declare function RadioGroup(props: RadioGroupProps): JSX.Element;
export declare namespace RadioGroup {
  function Radio(props: {value: string; description?: ReactNode; children?: ReactNode}): JSX.Element;
}
