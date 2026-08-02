import React from 'react';
import {ColorSwatch} from './ColorSwatch.jsx';

/** Hex entry with a live swatch. */
export function ColorField({value = '#2E6FF2', onChange, className = '', ...rest}) {
  return (
    <div className={['color-field', className].filter(Boolean).join(' ')} {...rest}>
      <ColorSwatch color={value} size={20} />
      <input className="color-field__input" value={value} onChange={e => onChange?.(e.target.value)} spellCheck={false} />
    </div>
  );
}
