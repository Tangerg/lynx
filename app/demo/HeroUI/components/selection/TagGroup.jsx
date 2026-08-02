import React from 'react';
import {Icon} from '../content/Icon.jsx';

/** Row of removable / selectable tags. */
export function TagGroup({className = '', children, ...rest}) {
  return <div className={['tag-group', className].filter(Boolean).join(' ')} {...rest}>{children}</div>;
}
TagGroup.Tag = function Tag({isSelected, onRemove, className = '', children, ...rest}) {
  return (
    <span className={['tag', className].filter(Boolean).join(' ')} data-selected={isSelected ? 'true' : undefined} {...rest}>
      {children}
      {onRemove && <button type="button" className="tag__remove" aria-label="Remove" onClick={onRemove}><Icon name="x" size={12} /></button>}
    </span>
  );
};
