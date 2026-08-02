import type {HTMLAttributes, ReactNode} from 'react';

export interface BreadcrumbItem {
  label: ReactNode;
  href?: string;
}

/** Path back up the hierarchy. */
export interface BreadcrumbsProps extends HTMLAttributes<HTMLElement> {
  items?: BreadcrumbItem[];
}
export declare function Breadcrumbs(props: BreadcrumbsProps): JSX.Element;
