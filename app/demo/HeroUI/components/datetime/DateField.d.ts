import type {HTMLAttributes} from 'react';

/** Segmented date input. (v2's `DateInput`.) */
export interface DateFieldProps extends HTMLAttributes<HTMLDivElement> {
  /** `MM/DD/YYYY`; empty segments show their placeholder. */
  value?: string;
}
export declare function DateField(props: DateFieldProps): JSX.Element;
