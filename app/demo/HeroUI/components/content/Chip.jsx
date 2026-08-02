import React from 'react';
import {Icon} from './Icon.jsx';

/** Compact label. Combine a size with a color: .chip--sm.chip--success. */
export function Chip({color, size = 'md', variant, dot = false, onRemove, className = '', children, ...rest}) {
  return (
    <span className={['chip', size !== 'md' && `chip--${size}`, color && `chip--${color}`, variant === 'outline' && 'chip--outline', className].filter(Boolean).join(' ')} {...rest}>
      {dot && <span className="chip__dot" />}
      <span className="chip__label">{children}</span>
      {onRemove && <button type="button" className="chip__close" onClick={onRemove} aria-label="Remove"><Icon name="x" size={11} /></button>}
    </span>
  );
}
