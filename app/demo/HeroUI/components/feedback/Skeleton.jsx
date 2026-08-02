import React from 'react';

/** Shimmering placeholder. */
export function Skeleton({variant = 'block', width, height, className = '', style, ...rest}) {
  return <span className={['skeleton', variant !== 'block' && `skeleton--${variant}`, className].filter(Boolean).join(' ')}
    style={{display: 'block', width, height, ...style}} {...rest} />;
}
