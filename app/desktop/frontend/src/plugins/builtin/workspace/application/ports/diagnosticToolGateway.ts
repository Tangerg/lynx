export interface InvokeDiagnosticToolInput {
  name: string;
  arguments: Record<string, unknown>;
  cwd?: string;
}

/** Consumer-owned boundary for one exact Runtime client's direct Tool invocation. */
export interface DiagnosticToolGateway {
  invoke(input: InvokeDiagnosticToolInput): Promise<unknown>;
}
