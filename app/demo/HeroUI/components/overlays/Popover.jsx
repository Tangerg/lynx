import React from 'react';

/** Anchored floating panel. */
export function Popover({trigger, placement = 'bottom-start', className = '', children, ...rest}) {
  const [open, setOpen] = React.useState(false);
  const align = placement.endsWith('end') ? {insetInlineEnd: 0} : {insetInlineStart: 0};
  return (
    <span style={{position: 'relative', display: 'inline-block'}}>
      <span onClick={() => setOpen(!open)}>{trigger}</span>
      {open && (
        <div className={['popover', className].filter(Boolean).join(' ')}
          style={{position: 'absolute', top: '100%', marginTop: 8, zIndex: 40, ...align}} {...rest}>{children}</div>
      )}
    </span>
  );
}
