import type {HTMLAttributes, ReactNode} from 'react';

/** Tray that turns ToggleButtons into a segmented switch. */
export interface ToggleButtonGroupProps extends HTMLAttributes<HTMLDivElement> {
  children?: ReactNode;
}
export declare function ToggleButtonGroup(props: ToggleButtonGroupProps): JSX.Element;
