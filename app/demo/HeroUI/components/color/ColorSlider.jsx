import React from 'react';

/** Hue or alpha track. */
export function ColorSlider({channel = 'hue', value = 0.5, color = '#2E6FF2', className = '', style, ...rest}) {
  const bg = channel === 'hue'
    ? 'linear-gradient(to right, #f00, #ff0, #0f0, #0ff, #00f, #f0f, #f00)'
    : `linear-gradient(to right, transparent, ${color})`;
  return (
    <div className={['color-slider', className].filter(Boolean).join(' ')} style={{background: bg, ...style}} {...rest}>
      <span className="color-slider__thumb" style={{insetInlineStart: `${value * 100}%`, background: color}} />
    </div>
  );
}
