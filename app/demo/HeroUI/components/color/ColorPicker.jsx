import React from 'react';
import {ColorArea} from './ColorArea.jsx';
import {ColorSlider} from './ColorSlider.jsx';
import {ColorField} from './ColorField.jsx';

/** Area + hue/alpha sliders + hex field on one overlay panel. */
export function ColorPicker({value = '#2E6FF2', hue = 220, onChange, showAlpha = true, className = '', ...rest}) {
  return (
    <div className={['color-picker', className].filter(Boolean).join(' ')} {...rest}>
      <ColorArea hue={hue} />
      <ColorSlider channel="hue" value={hue / 360} />
      {showAlpha && <ColorSlider channel="alpha" color={value} value={1} />}
      <ColorField value={value} onChange={onChange} />
    </div>
  );
}
