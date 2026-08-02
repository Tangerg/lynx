import React from 'react';

/** Saturation × brightness plane for a fixed hue. */
export function ColorArea({hue = 220, x = 0.7, y = 0.3, className = '', style, ...rest}) {
  return (
    <div className={['color-area', className].filter(Boolean).join(' ')}
      style={{background: `linear-gradient(to top, #000, transparent), linear-gradient(to right, #fff, hsl(${hue} 100% 50%))`, ...style}} {...rest}>
      <span className="color-area__thumb" style={{insetInlineStart: `${x * 100}%`, top: `${y * 100}%`, background: `hsl(${hue} 100% 50%)`}} />
    </div>
  );
}
