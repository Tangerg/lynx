import React from 'react';

/** Floating tray of icon actions. */
export function Toolbar({orientation = 'horizontal', className = '', children, ...rest}) {
  return <div role="toolbar" aria-orientation={orientation}
    className={['toolbar', orientation === 'vertical' && 'toolbar--vertical', className].filter(Boolean).join(' ')} {...rest}>{children}</div>;
}
Toolbar.Separator = function ToolbarSeparator(props) {
  return <div className="toolbar__separator" {...props} />;
};
