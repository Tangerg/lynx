import type {HTMLAttributes} from 'react';

/** Date entry with a calendar popover. `DateRangePicker` is the two-value form. */
export interface DatePickerProps extends HTMLAttributes<HTMLSpanElement> {
  /** `MM/DD/YYYY`. */
  value?: string;
  onSelect?: (day: number) => void;
}
export declare function DatePicker(props: DatePickerProps): JSX.Element;
