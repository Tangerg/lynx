import type {HTMLAttributes} from 'react';

/** Page selector for paged lists and tables. */
export interface PaginationProps extends Omit<HTMLAttributes<HTMLElement>, 'onChange'> {
  /** @default 1 */
  total?: number;
  /** 1-based current page. @default 1 */
  page?: number;
  /** Pages shown either side of the current one. @default 1 */
  siblings?: number;
  onChange?: (page: number) => void;
}
export declare function Pagination(props: PaginationProps): JSX.Element;
