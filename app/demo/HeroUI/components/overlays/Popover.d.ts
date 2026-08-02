import type {HTMLAttributes, ReactNode} from 'react';

/** Rich floating panel anchored to a trigger. */
export interface PopoverProps extends HTMLAttributes<HTMLDivElement> {
  trigger?: ReactNode;
  /** @default 'bottom-start' */
  placement?: 'bottom-start' | 'bottom-end';
  children?: ReactNode;
}
export declare function Popover(props: PopoverProps): JSX.Element;
