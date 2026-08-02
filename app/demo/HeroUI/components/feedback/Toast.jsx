import React from 'react';
import {CloseButton} from '../buttons/CloseButton.jsx';

/** Transient overlay notification. */
export function Toast({title, icon, action, onClose, className = '', children, ...rest}) {
  return (
    <div role="status" className={['toast', className].filter(Boolean).join(' ')} {...rest}>
      {icon && <span className="alert__icon">{icon}</span>}
      <span className="toast__content">
        {title && <span className="toast__title">{title}</span>}
        {children && <span className="toast__description">{children}</span>}
      </span>
      {action}
      {onClose && <CloseButton size="sm" onClick={onClose} />}
    </div>
  );
}
Toast.Region = function ToastRegion({className = '', children, ...rest}) {
  return <div className={['toast-region', className].filter(Boolean).join(' ')} {...rest}>{children}</div>;
};
