import React from 'react';

/** Hairline rule. Replaces v2's Divider. */
export function Separator({orientation = 'horizontal', level, className = '', ...rest}) {
  return <div role="separator" aria-orientation={orientation}
    className={['separator', `separator--${orientation}`, level && `separator--${level}`, className].filter(Boolean).join(' ')} {...rest} />;
}
