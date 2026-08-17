export interface CodebaseSearchHit {
  path: string;
  startLine: number;
  endLine: number;
  snippet: string;
  score: number;
}

export interface CodebaseReindexOperation {
  operationId: string;
}

export interface CodebaseGateway {
  search(input: {
    cwd: string | undefined;
    query: string;
    limit: number;
  }): Promise<CodebaseSearchHit[]>;
  reindex(cwd: string | undefined): Promise<CodebaseReindexOperation>;
}
