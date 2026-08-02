import type {HTMLAttributes} from 'react';

/** Segmented time input. (v2's `TimeInput`.) */
export interface TimeFieldProps extends HTMLAttributes<HTMLDivElement> {
  /** `HH:MM AM`. */
  value?: string;
}
export declare function TimeField(props: TimeFieldProps): JSX.Element;
