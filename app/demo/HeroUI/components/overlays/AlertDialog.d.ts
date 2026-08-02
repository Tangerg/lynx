import type {ReactNode} from 'react';

/** Blocking confirmation for destructive actions. */
export interface AlertDialogProps {
  /** @default true */
  isOpen?: boolean;
  onCancel?: () => void;
  onConfirm?: () => void;
  title?: ReactNode;
  description?: ReactNode;
  /** @default 'Delete' */
  confirmLabel?: string;
  /** @default 'Cancel' */
  cancelLabel?: string;
  /** @default true */
  isDestructive?: boolean;
}
export declare function AlertDialog(props: AlertDialogProps): JSX.Element;
