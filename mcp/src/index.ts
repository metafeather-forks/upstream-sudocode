#!/usr/bin/env node

/**
 * sudocode MCP Server entry point
 */

import { SudocodeMCPServer } from "./server.js";
import { SudocodeMCPServerConfig } from "./types.js";
import * as path from "path";
import {
  resolveScopes,
  hasExtendedScopes,
  getMissingServerUrlScopes,
} from "./scopes.js";
import {
  parseArgs,
} from "./cli-utils.js";
import {
  resolveProjectById,
  resolveProjectPath,
  discoverProject,
} from "@sudocode-ai/cli/project-discovery";

function showHelp(): void {
  console.log(`
sudocode MCP Server

Usage: sudocode-mcp [options]

Options:
  --project-id <id>         Project ID from registry (required for project-bound operations)
  -d, --sudocode-dir <path> Override sudocode data directory (resolved from project-id by default)
  --cli-path <path>         Path to sudocode CLI (default: 'sudocode' or SUDOCODE_PATH)
  --no-sync                 Skip initial sync on startup (default: sync enabled)
  -s, --scope <scopes>      Comma-separated list of scopes to enable (default: "default")
  --server-url <url>        Local server URL for extended tools (required if scope != default)
  -h, --help                Show this help message

Scopes:
  default                   Original 10 CLI-wrapped tools (no server required)
  overview                  project_status tool
  executions                Execution management (list, show, start, follow-up, cancel)
  executions:read           Read-only execution tools (list, show)
  executions:write          Write execution tools (start, follow-up, cancel)
  inspection                Execution inspection (trajectory, changes, chain)
  workflows                 Workflow orchestration (list, show, status, create, control)
  workflows:read            Read-only workflow tools
  workflows:write           Write workflow tools
  escalation                User communication (escalate, notify)

Meta-scopes:
  project-assistant         All extended scopes (overview, executions, inspection, workflows, escalation)
  all                       default + project-assistant

Examples:
  # Project-bound operation (required --project-id)
  sudocode-mcp --project-id my-project-abc12345

  # Enable execution monitoring
  sudocode-mcp --project-id my-project-abc12345 --scope default,executions:read --server-url http://localhost:3000

  # Full project assistant mode
  sudocode-mcp --project-id my-project-abc12345 --scope all --server-url http://localhost:3000

  # Find your project ID
  sudocode config project-id /path/to/repo

Environment Variables:
  SUDOCODE_PATH             Default CLI path
  SUDOCODE_SERVER_URL       Default server URL for extended tools
      `);
}

/**
 * Validate configuration and resolve scopes.
 * Requires --project-id for project-bound operations.
 */
function validateConfig(config: SudocodeMCPServerConfig): void {
  // Default scope if not specified
  const scopeArg = config.scope || "default";

  // Use env var for server URL if not specified
  if (!config.serverUrl && process.env.SUDOCODE_SERVER_URL) {
    config.serverUrl = process.env.SUDOCODE_SERVER_URL;
  }

  // Require --project-id for project-bound operations, or auto-discover
  if (!config.projectId) {
    // Auto-discover from cwd
    const discovery = discoverProject(process.cwd());
    if (discovery.source !== "generated") {
      config.projectId = discovery.projectId;
      if (!config.sudocodeDir) config.sudocodeDir = discovery.sudocodeDir;
      if (!config.workingDir) config.workingDir = discovery.projectPath;
      if (!config.dbPath) config.dbPath = path.join(discovery.sudocodeDir, "cache.db");
      console.error(`[mcp] Auto-discovered project: id=${discovery.projectId}, source=${discovery.source}`);
    } else {
      console.error(
        "Error: --project-id is required for project-bound MCP operations.\n" +
        "  No .sudocode found in current or parent directories.\n" +
        "  Use 'sudocode config project-id [path]' to find your project ID.\n" +
        "  Use 'sudocode init' to create a new project."
      );
      process.exit(1);
    }
  }

  // Resolve project from registry
  const resolved = resolveProjectById(config.projectId);
  if (!resolved) {
    console.error(
      `Error: Project not found in registry: ${config.projectId}\n` +
      "  Use 'sudocode config project-id [path]' to find valid project IDs.\n" +
      "  Use 'sudocode init' to register a new project."
    );
    process.exit(1);
  }

  // Set sudocodeDir from registry if not explicitly overridden
  if (!config.sudocodeDir) {
    config.sudocodeDir = resolved.sudocodeDir;
  }

  // Set workingDir from registry for CLI operations
  if (!config.workingDir) {
    const resolvedPath = resolveProjectPath(resolved.sudocodeDir);
    if (resolvedPath) {
      config.workingDir = resolvedPath;
    }
  }

  // Set dbPath from registry
  if (!config.dbPath) {
    config.dbPath = resolved.dbPath;
  }

  const resolvedPathForLog = resolveProjectPath(resolved.sudocodeDir) || "(unknown)";
  console.error(`[mcp] Resolved project: id=${resolved.projectId}, path=${resolvedPathForLog}`);

  try {
    // Validate and resolve scopes
    const scopeConfig = resolveScopes(
      scopeArg,
      config.serverUrl,
      config.projectId
    );

    // Check if extended scopes are enabled without server URL
    if (hasExtendedScopes(scopeConfig.enabledScopes) && !config.serverUrl) {
      const missingScopes = getMissingServerUrlScopes(
        scopeConfig.enabledScopes
      );
      console.error("");
      console.error(
        `⚠️  WARNING: Extended scopes require --server-url to be configured`
      );
      console.error(
        `   The following scopes will be disabled: ${missingScopes.join(", ")}`
      );
      console.error(`   Only 'default' scope tools will be available.`);
      console.error("");
    }
  } catch (error) {
    console.error(
      `Error: ${error instanceof Error ? error.message : String(error)}`
    );
    process.exit(1);
  }
}

async function main() {
  const config = parseArgs(process.argv) as SudocodeMCPServerConfig & {
    _showHelp?: boolean;
    _unknownOption?: string;
  };
  
  if (config._showHelp) {
    showHelp();
    process.exit(0);
  }
  
  if (config._unknownOption) {
    console.error(`Unknown option: ${config._unknownOption}`);
    console.error("Use --help for usage information");
    process.exit(1);
  }
  
  validateConfig(config);
  const server = new SudocodeMCPServer(config);
  await server.run();
}

main().catch((error) => {
  console.error("Fatal error:", error);
  process.exit(1);
});
