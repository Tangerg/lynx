import React from 'react';
import {Icon} from '../content/Icon.jsx';

/** Input with a leading search glyph. */
export function SearchField({className = '', ...rest}) {
  return (
    <div className={['search-field', className].filter(Boolean).join(' ')}>
      <span className="search-field__icon"><Icon name="search" /></span>
      <input type="search" className="input" placeholder="Search" {...rest} />
    </div>
  );
}
