import type {HTMLAttributes} from 'react';

/** Month grid for date selection. Pass `rangeEnd` for range behaviour. */
export interface CalendarProps extends HTMLAttributes<HTMLDivElement> {
  /** 0-indexed month. Defaults to the current month. */
  month?: number;
  year?: number;
  /** Selected day of month (or range start). */
  selected?: number;
  /** Range end — turns the grid into a RangeCalendar. */
  rangeEnd?: number;
  onSelect?: (day: number) => void;
}
export declare function Calendar(props: CalendarProps): JSX.Element;
