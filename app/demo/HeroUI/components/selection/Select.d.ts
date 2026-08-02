import type {HTMLAttributes, ReactNode} from 'react';

export interface SelectOption {
  value: string;
  label: ReactNode;
  description?: ReactNode;
}

/**
 * Closed-set choice with a popover list.
 * @startingPoint section="Forms" subtitle="Select trigger with popover option list" viewport="700x260"
 */
export interface SelectProps extends Omit<HTMLAttributes<HTMLDivElement>, 'onChange' | 'defaultValue'> {
  options?: SelectOption[];
  value?: string;
  defaultValue?: string;
  /** @default 'Select…' */
  placeholder?: string;
  onChange?: (value: string) => void;
}
export declare function Select(props: SelectProps): JSX.Element;
