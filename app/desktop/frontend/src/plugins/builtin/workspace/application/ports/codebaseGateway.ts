import { createSingletonPort } from "@/lib/ports/singletonPort";
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

const port = createSingletonPort<CodebaseGateway>("Codebase gateway is not configured");

export const configureCodebaseGateway = port.configure;
export const codebaseGateway = port.get;
