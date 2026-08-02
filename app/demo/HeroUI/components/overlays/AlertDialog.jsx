import React from 'react';
import {Icon} from '../content/Icon.jsx';
import {Modal} from './Modal.jsx';
import {Button} from '../buttons/Button.jsx';

/** Confirmation dialog for destructive or irreversible actions. */
export function AlertDialog({isOpen = true, onCancel, onConfirm, title, description, confirmLabel = 'Delete', cancelLabel = 'Cancel', isDestructive = true}) {
  return (
    <Modal isOpen={isOpen} onClose={onCancel} size="sm" className="alert-dialog">
      <Modal.Content style={{paddingTop: '1.25rem'}}>
        <span className="alert-dialog__icon"><Icon name="alert-triangle" size={18} /></span>
        <h2 className="modal__title">{title}</h2>
        {description && <p className="modal__description">{description}</p>}
      </Modal.Content>
      <Modal.Footer>
        <Button variant="ghost" onClick={onCancel}>{cancelLabel}</Button>
        <Button variant={isDestructive ? 'danger' : 'primary'} onClick={onConfirm}>{confirmLabel}</Button>
      </Modal.Footer>
    </Modal>
  );
}
