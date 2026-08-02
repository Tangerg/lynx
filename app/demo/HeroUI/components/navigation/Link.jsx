import React from 'react';
import {Icon} from '../content/Icon.jsx';

/** Inline or standalone link. */
export function Link({variant, isExternal = false, className = '', children, ...rest}) {
  return (
    <a className={['link', variant && `link--${variant}`, className].filter(Boolean).join(' ')}
      target={isExternal ? '_blank' : undefined} rel={isExternal ? 'noreferrer' : undefined} {...rest}>
      {children}
      {isExternal && <span data-slot="link-icon" className="link__icon"><Icon name="arrow-up-right" size={13} /></span>}
    </a>
  );
}
