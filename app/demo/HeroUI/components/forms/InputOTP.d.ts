import type {HTMLAttributes} from 'react';

/** One-time passcode field rendered as separate digit slots. */
export interface InputOTPProps extends HTMLAttributes<HTMLDivElement> {
  /** @default 6 */
  length?: number;
  /** Digits entered so far. */
  value?: string;
  /** Slot that shows the blinking caret. @default 0 */
  activeIndex?: number;
}
export declare function InputOTP(props: InputOTPProps): JSX.Element;
