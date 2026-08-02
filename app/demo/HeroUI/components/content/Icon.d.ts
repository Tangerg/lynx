import type {SVGProps} from 'react';

/** Stroke icon from the bundled Lucide subset — 24px grid, 2px round stroke. */
export interface IconProps extends Omit<SVGProps<SVGSVGElement>, 'name'> {
  /** Lucide glyph id, e.g. "chevron-down", "check", "search". */
  name: string;
  /** Rendered square size in px. Buttons use 16, standalone UI 20. @default 16 */
  size?: number;
  /** @default 2 */
  strokeWidth?: number;
  className?: string;
}
export declare function Icon(props: IconProps): JSX.Element | null;
export declare const iconNames: string[];
