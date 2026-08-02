import type {HTMLAttributes, ReactNode} from 'react';

/** Continuous value selection. */
export interface SliderProps extends Omit<HTMLAttributes<HTMLDivElement>, 'onChange' | 'defaultValue'> {
  label?: ReactNode;
  value?: number;
  /** @default 50 */
  defaultValue?: number;
  /** @default 0 */
  min?: number;
  /** @default 100 */
  max?: number;
  /** @default 1 */
  step?: number;
  /** Formats the readout, e.g. `v => \`${v}%\``. */
  formatValue?: (value: number) => string;
  onChange?: (value: number) => void;
}
export declare function Slider(props: SliderProps): JSX.Element;
