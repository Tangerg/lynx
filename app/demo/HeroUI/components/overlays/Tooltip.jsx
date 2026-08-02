import React from 'react';

/** Short hover hint. Inverted chip, appears above the trigger. */
export function Tooltip({content, shortcut, placement = 'top', children, className = '', ...rest}) {
  const [open, setOpen] = React.useState(false);
  const pos = placement === 'bottom'
    ? {top: '100%', marginTop: 6} : {bottom: '100%', marginBottom: 6};
  return (
    <span style={{position: 'relative', display: 'inline-flex'}}
      onMouseEnter={() => setOpen(true)} onMouseLeave={() => setOpen(false)} onFocus={() => setOpen(true)} onBlur={() => setOpen(false)}>
      {children}
      {open && (
        <span role="tooltip" className={['tooltip', className].filter(Boolean).join(' ')}
          style={{position: 'absolute', left: '50%', translate: '-50% 0', zIndex: 45, ...pos}} {...rest}>
          {content}
          {shortcut && <span className="kbd tooltip__kbd">{shortcut}</span>}
        </span>
      )}
    </span>
  );
}
