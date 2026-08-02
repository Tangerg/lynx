import type {HTMLAttributes, ReactNode} from 'react';

/** Vertical list of expandable sections sharing one surface. */
export interface AccordionProps extends HTMLAttributes<HTMLDivElement> {
  /** Hairline outline instead of a filled surface. */
  variant?: 'default' | 'outline';
  /** @default false */
  allowMultiple?: boolean;
  defaultExpandedKeys?: string[];
  children?: ReactNode;
}
export declare function Accordion(props: AccordionProps): JSX.Element;
export declare namespace Accordion {
  function Item(props: {id: string; title?: ReactNode; children?: ReactNode} & HTMLAttributes<HTMLDivElement>): JSX.Element;
}
