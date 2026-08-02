import React from 'react';
import {Label} from './Label.jsx';
import {Description} from './Description.jsx';

/** Stacked (or inline) set of Checkboxes with a shared label. */
export function CheckboxGroup({label, description, orientation = 'vertical', className = '', children, ...rest}) {
  return (
    <div className={['field', className].filter(Boolean).join(' ')} role="group" {...rest}>
      {label && <Label>{label}</Label>}
      <div className={['checkbox-group', orientation === 'horizontal' && 'checkbox-group--horizontal'].filter(Boolean).join(' ')}>{children}</div>
      {description && <Description>{description}</Description>}
    </div>
  );
}
