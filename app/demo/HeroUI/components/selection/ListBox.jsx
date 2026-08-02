import React from 'react';
import {Icon} from '../content/Icon.jsx';

/** Overlay list of choices. ListBox.Item / ListBox.Section. */
export function ListBox({className = '', children, ...rest}) {
  return <div role="listbox" className={['list-box', className].filter(Boolean).join(' ')} {...rest}>{children}</div>;
}
ListBox.Item = function ListBoxItem({isSelected, isDisabled, startContent, description, showCheck = false, className = '', children, ...rest}) {
  return (
    <div role="option" aria-selected={!!isSelected} tabIndex={-1}
      data-selected={isSelected ? 'true' : undefined} data-disabled={isDisabled ? 'true' : undefined}
      className={['list-box__item', className].filter(Boolean).join(' ')} {...rest}>
      {startContent}
      <span style={{display: 'flex', flexDirection: 'column', minWidth: 0}}>
        <span>{children}</span>
        {description && <span className="list-box__item-description">{description}</span>}
      </span>
      {showCheck && isSelected && <span className="list-box__check"><Icon name="check" /></span>}
    </div>
  );
};
ListBox.Section = function ListBoxSection({label, children}) {
  return <div role="group">{label && <div className="list-box__section-label">{label}</div>}{children}</div>;
};
