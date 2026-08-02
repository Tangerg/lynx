import React from 'react';
import {Icon} from '../content/Icon.jsx';

/** Ghost × for dismissing overlays, chips and banners. */
export function CloseButton({size = 'md', className = '', label = 'Close', ...rest}) {
  return (
    <button type="button" aria-label={label}
      className={['close-button', size !== 'md' && `close-button--${size}`, className].filter(Boolean).join(' ')} {...rest}>
      <Icon name="x" size={size === 'lg' ? 18 : 15} />
    </button>
  );
}
