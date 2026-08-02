import React from 'react';

const TAGS = {h1: 'h1', h2: 'h2', h3: 'h3', h4: 'h4', h5: 'h5', h6: 'h6', body: 'p', 'body-sm': 'p', 'body-xs': 'p', code: 'code'};

/** Semantic type primitive. Typography.Heading / Paragraph / Code / Prose. */
export function Typography({type = 'body', align, color, weight, truncate, as, className = '', children, ...rest}) {
  const Tag = as || TAGS[type] || 'p';
  const cls = ['typography', `typography--${type}`,
    align && `typography--align-${align}`, color && `typography--color-${color}`,
    weight && `typography--weight-${weight}`, truncate && 'typography--truncate', className]
    .filter(Boolean).join(' ');
  return <Tag className={cls} {...rest}>{children}</Tag>;
}
Typography.Heading = function Heading({level = 2, ...rest}) {
  return <Typography type={`h${level}`} {...rest} />;
};
Typography.Paragraph = function Paragraph({size = 'base', ...rest}) {
  return <Typography type={size === 'base' ? 'body' : `body-${size}`} {...rest} />;
};
Typography.Code = function Code(props) { return <Typography type="code" {...props} />; };
Typography.Prose = function Prose({className = '', children, ...rest}) {
  return <div className={['typography-prose', className].filter(Boolean).join(' ')} {...rest}>{children}</div>;
};
