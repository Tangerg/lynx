export interface CodebaseSearchHit {
  path: string;
  startLine: number;
  endLine: number;
  snippet: string;
  score: number;
}

export interface CodebaseGateway {
  search(input: {
    cwd: string | undefined;
    query: string;
    limit: number;
  }): Promise<CodebaseSearchHit[]>;
  reindex(cwd: string | undefined): Promise<void>;
}

let port: CodebaseGateway | null = null;

export function configureCodebaseGateway(next: CodebaseGateway): void {
  port = next;
}

export function codebaseGateway(): CodebaseGateway {
  if (!port) throw new Error("Codebase gateway is not configured");
  return port;
}
