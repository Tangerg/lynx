import React from 'react';
import {CloseButton} from '../buttons/CloseButton.jsx';

/** Centred dialog on a blurred scrim. */
export function Modal({isOpen = true, onClose, size = 'md', className = '', children, ...rest}) {
  if (!isOpen) return null;
  return (
    <div className="backdrop" onClick={onClose}>
      <div role="dialog" aria-modal="true" onClick={e => e.stopPropagation()}
        className={['modal', size !== 'md' && `modal--${size}`, className].filter(Boolean).join(' ')} {...rest}>
        {children}
      </div>
    </div>
  );
}
Modal.Header = function ModalHeader({title, description, onClose, className = '', children, ...rest}) {
  return (
    <div className={['modal__header', className].filter(Boolean).join(' ')} {...rest}>
      <div>
        {title && <h2 className="modal__title">{title}</h2>}
        {description && <p className="modal__description">{description}</p>}
        {children}
      </div>
      {onClose && <CloseButton onClick={onClose} />}
    </div>
  );
};
Modal.Content = function ModalContent({className = '', children, ...rest}) {
  return <div className={['modal__content', className].filter(Boolean).join(' ')} {...rest}>{children}</div>;
};
Modal.Footer = function ModalFooter({className = '', children, ...rest}) {
  return <div className={['modal__footer', className].filter(Boolean).join(' ')} {...rest}>{children}</div>;
};
