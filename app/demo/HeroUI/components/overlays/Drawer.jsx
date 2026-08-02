import React from 'react';

/** Edge-anchored panel. */
export function Drawer({isOpen = true, onClose, placement = 'right', className = '', children, ...rest}) {
  if (!isOpen) return null;
  return (
    <>
      <div className="backdrop" onClick={onClose} style={{background: 'var(--backdrop)'}} />
      <aside role="dialog" aria-modal="true" className={['drawer', `drawer--${placement}`, className].filter(Boolean).join(' ')} {...rest}>
        {placement === 'bottom' && <span className="drawer__handle" />}
        {children}
      </aside>
    </>
  );
}
Drawer.Header = function DrawerHeader(props) { return <div className="modal__header" {...props} />; };
Drawer.Content = function DrawerContent(props) { return <div className="modal__content" {...props} />; };
Drawer.Footer = function DrawerFooter(props) { return <div className="modal__footer" {...props} />; };
