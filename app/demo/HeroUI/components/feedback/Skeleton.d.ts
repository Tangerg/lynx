import type {HTMLAttributes} from 'react';

/** Loading placeholder with the shared shimmer keyframe. */
export interface SkeletonProps extends HTMLAttributes<HTMLSpanElement> {
  /** @default 'block' */
  variant?: 'block' | 'text' | 'circle';
  width?: number | string;
  height?: number | string;
}
export declare function Skeleton(props: SkeletonProps): JSX.Element;
