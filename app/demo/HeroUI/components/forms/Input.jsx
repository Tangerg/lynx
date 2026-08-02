import React from 'react';

/** Bare text input. HeroUI fields are borderless — fill + shadow only. */
export function Input({size = 'md', isInvalid = false, className = '', ...rest}) {
  return <input className={['input', size !== 'md' && `input--${size}`, isInvalid && 'input--invalid', className].filter(Boolean).join(' ')} aria-invalid={isInvalid || undefined} {...rest} />;
}
