import React from 'react';
import {Icon} from '../content/Icon.jsx';
import {CloseButton} from '../buttons/CloseButton.jsx';

const ICONS = {accent: 'info', success: 'circle-check', warning: 'alert-triangle', danger: 'circle-alert'};

/** Inline status banner. */
export function Alert({color, title, icon, onClose, className = '', children, ...rest}) {
  const glyph = icon !== undefined ? icon : <Icon name={ICONS[color] || 'info'} size={18} />;
  return (
    <div role="alert" className={['alert', color && `alert--${color}`, className].filter(Boolean).join(' ')} {...rest}>
      {glyph && <span className="alert__icon">{glyph}</span>}
      <span className="alert__content">
        {title && <span className="alert__title">{title}</span>}
        {children && <span className="alert__description">{children}</span>}
      </span>
      {onClose && <CloseButton size="sm" onClick={onClose} />}
    </div>
  );
}
