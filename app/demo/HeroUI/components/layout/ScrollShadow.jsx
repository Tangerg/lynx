import React from 'react';

/** Overflow container that fades its scrollable edges with a mask. */
export function ScrollShadow({orientation = 'vertical', size = 40, className = '', style, children, ...rest}) {
  const ref = React.useRef(null);
  const [edges, setEdges] = React.useState({start: false, end: false});
  const measure = React.useCallback(() => {
    const el = ref.current; if (!el) return;
    const vertical = orientation === 'vertical';
    const pos = vertical ? el.scrollTop : el.scrollLeft;
    const max = (vertical ? el.scrollHeight - el.clientHeight : el.scrollWidth - el.clientWidth);
    setEdges({start: pos > 2, end: pos < max - 2});
  }, [orientation]);
  React.useEffect(() => { measure(); }, [measure, children]);
  const both = edges.start && edges.end;
  const attrs = orientation === 'vertical'
    ? {'data-top-scroll': edges.start && !both ? 'true' : undefined,
       'data-bottom-scroll': edges.end && !both ? 'true' : undefined,
       'data-top-bottom-scroll': both ? 'true' : undefined}
    : {'data-left-right-scroll': both ? 'true' : undefined};
  return (
    <div ref={ref} onScroll={measure} {...attrs}
      className={['scroll-shadow', `scroll-shadow--${orientation}`, className].filter(Boolean).join(' ')}
      style={{'--scroll-shadow-size': `${size}px`, ...style}} {...rest}>{children}</div>
  );
}
