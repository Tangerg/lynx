import React from 'react';
import {Icon} from '../content/Icon.jsx';

/** Stacked disclosure list on one surface. */
export function Accordion({variant, allowMultiple = false, defaultExpandedKeys = [], className = '', children, ...rest}) {
  const [open, setOpen] = React.useState(new Set(defaultExpandedKeys));
  const toggle = k => setOpen(prev => {
    const next = new Set(allowMultiple ? prev : []);
    if (prev.has(k)) next.delete(k); else next.add(k);
    return next;
  });
  return (
    <div className={['accordion', variant === 'outline' && 'accordion--outline', className].filter(Boolean).join(' ')} {...rest}>
      {React.Children.map(children, c => React.isValidElement(c)
        ? React.cloneElement(c, {__expanded: open.has(c.props.id), __toggle: () => toggle(c.props.id)}) : c)}
    </div>
  );
}
Accordion.Item = function AccordionItem({id, title, __expanded, __toggle, className = '', children, ...rest}) {
  return (
    <div className={['accordion__item', className].filter(Boolean).join(' ')} data-expanded={__expanded ? 'true' : undefined} {...rest}>
      <button type="button" className="accordion__trigger" aria-expanded={!!__expanded} onClick={__toggle}>
        <span>{title}</span>
        <span className="accordion__indicator"><Icon name="chevron-down" /></span>
      </button>
      {__expanded && <div className="accordion__panel">{children}</div>}
    </div>
  );
};
