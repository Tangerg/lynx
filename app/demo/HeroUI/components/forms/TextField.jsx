import React from 'react';
import {Input} from './Input.jsx';
import {Label} from './Label.jsx';
import {Description} from './Description.jsx';
import {FieldError} from './FieldError.jsx';

/** Label + input + description/error as one block. */
export function TextField({label, description, errorMessage, isRequired, id, className = '', children, ...inputProps}) {
  const auto = React.useId();
  const fieldId = id || auto;
  return (
    <div className={['field', className].filter(Boolean).join(' ')}>
      {label && <Label htmlFor={fieldId} isRequired={isRequired}>{label}</Label>}
      {children || <Input id={fieldId} isInvalid={!!errorMessage} required={isRequired} {...inputProps} />}
      {errorMessage ? <FieldError>{errorMessage}</FieldError> : description ? <Description>{description}</Description> : null}
    </div>
  );
}
