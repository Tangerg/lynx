import React from 'react';

/** Colour chip on a checkerboard so alpha reads correctly. */
export function ColorSwatch({color, shape = 'rounded', size = 32, className = '', style, ...rest}) {
  return (
    <span className={['color-swatch', shape === 'circle' && 'color-swatch--circle', className].filter(Boolean).join(' ')}
      style={{width: size, height: size, ...style}} title={color} {...rest}>
      <span className="color-swatch__fill" style={{background: color}} />
    </span>
  );
}
