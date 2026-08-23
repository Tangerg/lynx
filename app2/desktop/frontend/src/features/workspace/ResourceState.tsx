interface ResourceStateProps {
  title: string;
  detail?: string;
  action?: string;
  onAction?(): void;
}

export function ResourceState(props: ResourceStateProps) {
  return (
    <div className="resource-state">
      <h4>{props.title}</h4>
      {props.detail ? <p>{props.detail}</p> : null}
      {props.action && props.onAction ? (
        <button type="button" onClick={props.onAction}>
          {props.action}
        </button>
      ) : null}
    </div>
  );
}
