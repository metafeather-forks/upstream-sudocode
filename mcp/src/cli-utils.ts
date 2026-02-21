/**
 * CLI utility functions for sudocode MCP Server
 * 
 * These functions are extracted for testability.
 */

import { createHash } from "crypto";
import { basename, resolve } from "path";
import { SudocodeMCPServerConfig } from "./types.js";

/**
 * Generate a deterministic project ID from a path.
 * Uses the same algorithm as the server's ProjectRegistry.
 * Format: <repo-name>-<8-char-hash>
 */
export function generateProjectId(projectPath: string): string {
  // Resolve to absolute path
  const absolutePath = resolve(projectPath);

  // Extract repo name from path
  const repoName = basename(absolutePath);

  // Create URL-safe version of repo name
  const safeName = repoName
    .toLowerCase()
    .replace(/[^a-z0-9-]/g, "-")
    .replace(/-+/g, "-")
    .replace(/^-|-$/g, "") // Remove leading/trailing dashes
    .slice(0, 32);

  // Generate short hash for uniqueness
  const hash = createHash("sha256").update(absolutePath).digest("hex").slice(0, 8);

  return `${safeName}-${hash}`;
}

/**
 * Fix unexpanded ${workspaceFolder} or other VS Code variables.
 * Returns the fixed path, or the original if no fix needed.
 */
export function fixUnexpandedVariables(workingDir: string | undefined): string | undefined {
  if (!workingDir) return workingDir;
  
  // Detect unexpanded VS Code variables
  if (workingDir === "${workspaceFolder}" || workingDir.includes("${")) {
    const fixed = process.env.PWD || process.cwd();
    console.error(`[mcp] Fixed unexpanded variable: "${workingDir}" -> "${fixed}"`);
    return fixed;
  }
  
  return workingDir;
}

/**
 * Parse CLI arguments into a config object.
 */
export function parseArgs(argv: string[]): SudocodeMCPServerConfig {
  const config: SudocodeMCPServerConfig = {};
  const args = argv.slice(2);

  for (let i = 0; i < args.length; i++) {
    const arg = args[i];

    switch (arg) {
      case "--working-dir":
      case "-w":
        config.workingDir = args[++i];
        break;
      case "--cli-path":
        config.cliPath = args[++i];
        break;
      case "--db-path":
      case "--db":
        config.dbPath = args[++i];
        break;
      case "--sudocode-dir":
      case "-d":
        config.sudocodeDir = args[++i];
        break;
      case "--no-sync":
        config.syncOnStartup = false;
        break;
      case "--scope":
      case "-s":
        config.scope = args[++i];
        break;
      case "--server-url":
        config.serverUrl = args[++i];
        break;
      case "--project-id":
        config.projectId = args[++i];
        break;
      case "--help":
      case "-h":
        // Return special marker for help (caller should handle)
        return { ...config, _showHelp: true } as any;
      default:
        // Return special marker for unknown option (caller should handle)
        return { ...config, _unknownOption: arg } as any;
    }
  }

  return config;
}
