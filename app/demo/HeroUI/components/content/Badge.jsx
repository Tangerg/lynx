import React from 'react';

/** Count or status dot pinned to the corner of its child. */
export function Badge({content, color = 'accent', dot = false, hidden = false, className = '', children, ...rest}) {
  return (
    <span className={['badge', className].filter(Boolean).join(' ')} {...rest}>
      {children}
      {!hidden && (
        <span className={['badge__indicator', color !== 'accent' && `badge__indicator--${color}`, dot && 'badge__indicator--dot'].filter(Boolean).join(' ')}>
          {dot ? null : content}
        </span>
      )}
    </span>
  );
}
