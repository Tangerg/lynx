import {
  cloneElement,
  useId,
  type HTMLAttributes,
  type ReactElement,
} from "react";

export function Tooltip(props: {
  label: string;
  shortcut?: readonly string[];
  children: ReactElement<HTMLAttributes<HTMLElement>>;
}) {
  const id = useId();
  const describedBy = [props.children.props["aria-describedby"], id]
    .filter((value): value is string => Boolean(value))
    .join(" ");
  const trigger = cloneElement(props.children, {
    "aria-describedby": describedBy,
  });
  return (
    <span className="tooltip-anchor">
      {trigger}
      <span className="tooltip-bubble" id={id} role="tooltip">
        <span>{props.label}</span>
        {props.shortcut ? (
          <span className="tooltip-shortcut" dir="ltr">
            {props.shortcut.map((token, index) => (
              <kbd key={`${token}:${index}`}>{token}</kbd>
            ))}
          </span>
        ) : null}
      </span>
    </span>
  );
}
