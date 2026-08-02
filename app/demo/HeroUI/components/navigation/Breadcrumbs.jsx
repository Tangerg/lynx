import React from 'react';
import {Icon} from '../content/Icon.jsx';

/** Hierarchy trail. */
export function Breadcrumbs({items = [], className = '', ...rest}) {
  return (
    <nav aria-label="Breadcrumb" className={['breadcrumbs', className].filter(Boolean).join(' ')} {...rest}>
      {items.map((item, i) => (
        <span key={i} className="breadcrumbs__item" data-current={i === items.length - 1 ? 'true' : undefined}>
          {i > 0 && <span className="breadcrumbs__separator"><Icon name="chevron-right" size={13} /></span>}
          {item.href && i !== items.length - 1
            ? <a className="breadcrumbs__link" href={item.href}>{item.label}</a>
            : <span>{item.label}</span>}
        </span>
      ))}
    </nav>
  );
}
