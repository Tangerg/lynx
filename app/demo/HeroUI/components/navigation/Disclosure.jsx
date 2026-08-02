import React from 'react';
import {Icon} from '../content/Icon.jsx';

/** A single expandable section. */
export function Disclosure({title, defaultExpanded = false, className = '', children, ...rest}) {
  const [open, setOpen] = React.useState(defaultExpanded);
  return (
    <div className={['disclosure', className].filter(Boolean).join(' ')} {...rest}>
      <button type="button" className="disclosure__trigger" aria-expanded={open} onClick={() => setOpen(!open)}>
        <span className="accordion__indicator" style={{rotate: open ? '0deg' : '-90deg'}}><Icon name="chevron-down" size={14} /></span>
        {title}
      </button>
      {open && <div className="disclosure__panel">{children}</div>}
    </div>
  );
}
