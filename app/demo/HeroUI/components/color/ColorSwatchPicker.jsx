import React from 'react';
import {ColorSwatch} from './ColorSwatch.jsx';

/** Pick from a fixed palette. */
export function ColorSwatchPicker({colors = [], value, onChange, size = 32, className = '', ...rest}) {
  return (
    <div className={['color-swatch-picker', className].filter(Boolean).join(' ')} role="radiogroup" {...rest}>
      {colors.map(c => (
        <button key={c} type="button" className="color-swatch-picker__item" aria-label={c}
          data-selected={c === value ? 'true' : undefined} onClick={() => onChange?.(c)}>
          <ColorSwatch color={c} size={size} />
        </button>
      ))}
    </div>
  );
}
