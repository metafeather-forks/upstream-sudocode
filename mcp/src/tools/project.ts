/**
 * MCP tools for project utilities
 */

import { SudocodeClient } from "../client.js";

// Tool parameter types
export interface GetProjectIdParams {
  path?: string;
}

export interface GetProjectIdResult {
  path: string;
  projectId: string;
}

/**
 * Get the project ID for a given path.
 * Uses the CLI's config project-id command.
 */
export async function getProjectId(
  client: SudocodeClient,
  params: GetProjectIdParams = {}
): Promise<GetProjectIdResult> {
  const args = ["config", "project-id"];

  if (params.path) {
    args.push(params.path);
  }

  // The CLI returns JSON with --json flag
  const result = await client.exec(args);
  return result as GetProjectIdResult;
}
