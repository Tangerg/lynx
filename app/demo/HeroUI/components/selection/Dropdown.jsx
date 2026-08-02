import React from 'react';
import {ListBox} from './ListBox.jsx';

/** Action menu anchored to a trigger. */
export function Dropdown({trigger, align = 'start', className = '', children, ...rest}) {
  const [open, setOpen] = React.useState(false);
  return (
    <div className={['dropdown', className].filter(Boolean).join(' ')} {...rest}>
      <span onClick={() => setOpen(!open)}>{trigger}</span>
      {open && (
        <div className="dropdown__menu" style={align === 'end' ? {insetInlineEnd: 0} : {insetInlineStart: 0}}
          onClick={() => setOpen(false)}>
          <ListBox>{children}</ListBox>
        </div>
      )}
    </div>
  );
}
Dropdown.Item = function DropdownItem({danger, shortcut, className = '', children, ...rest}) {
  return (
    <ListBox.Item className={[danger && 'dropdown__item--danger', className].filter(Boolean).join(' ')} {...rest}>
      {children}
      {shortcut && <span className="dropdown__shortcut">{shortcut}</span>}
    </ListBox.Item>
  );
};
Dropdown.Separator = function DropdownSeparator() { return <div className="dropdown__separator" />; };
