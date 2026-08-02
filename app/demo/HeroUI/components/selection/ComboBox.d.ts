import type {HTMLAttributes, ReactNode} from 'react';
import type {SelectOption} from './Select';

/** Filterable select — type to narrow, then pick. */
export interface ComboBoxProps extends Omit<HTMLAttributes<HTMLDivElement>, 'onSelect'> {
  options?: SelectOption[];
  /** @default 'Search…' */
  placeholder?: string;
  onSelect?: (value: string) => void;
}
export declare function ComboBox(props: ComboBoxProps): JSX.Element;
