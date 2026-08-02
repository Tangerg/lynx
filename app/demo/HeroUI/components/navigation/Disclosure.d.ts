import type {HTMLAttributes, ReactNode} from 'react';

/** Standalone show/hide section. */
export interface DisclosureProps extends HTMLAttributes<HTMLDivElement> {
  title?: ReactNode;
  /** @default false */
  defaultExpanded?: boolean;
  children?: ReactNode;
}
export declare function Disclosure(props: DisclosureProps): JSX.Element;
