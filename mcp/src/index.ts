#!/usr/bin/env node

/**
 * sudocode MCP Server entry point
 */

import { SudocodeMCPServer } from "./server.js";
import { SudocodeMCPServerConfig } from "./types.js";
import {
  resolveScopes,
  hasExtendedScopes,
  getMissingServerUrlScopes,
} from "./scopes.js";
import {
  fixUnexpandedVariables,
  parseArgs,
} from "./cli-utils.js";
import {
  discoverProject,
  generateProjectId,
} from "@sudocode-ai/cli/project-discovery";

function showHelp(): void {
  console.log(`
sudocode MCP Server

Usage: sudocode-mcp [options]

Options:
  -w, --working-dir <path>  Working directory (default: cwd or SUDOCODE_WORKING_DIR)
  -d, --sudocode-dir <path> Override sudocode data directory (default: from server or <working-dir>/.sudocode)
  --cli-path <path>         Path to sudocode CLI (default: 'sudocode' or SUDOCODE_PATH)
  --db-path <path>          Database path (default: auto-discover or SUDOCODE_DB)
  --no-sync                 Skip initial sync on startup (default: sync enabled)
  -s, --scope <scopes>      Comma-separated list of scopes to enable (default: "default")
  --server-url <url>        Local server URL for extended tools (required if scope != default)
  --project-id <id>         Project ID for API calls (auto-discovered from working dir)
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
  # Default behavior (original 10 tools)
  sudocode-mcp --working-dir /path/to/repo

  # Enable execution monitoring
  sudocode-mcp -w /path/to/repo --scope default,executions:read --server-url http://localhost:3000

  # Full project assistant mode
  sudocode-mcp -w /path/to/repo --scope all --server-url http://localhost:3000

Environment Variables:
  SUDOCODE_WORKING_DIR      Default working directory
  SUDOCODE_DIR              Fallback sudocode directory (used when server unavailable)
  SUDOCODE_PATH             Default CLI path
  SUDOCODE_DB               Default database path
  SUDOCODE_SERVER_URL       Default server URL for extended tools
      `);
}

/**
 * Validate configuration and resolve scopes.
 */
function validateConfig(config: SudocodeMCPServerConfig): void {
  // Default scope if not specified
  const scopeArg = config.scope || "default";

  // Use env var for server URL if not specified
  if (!config.serverUrl && process.env.SUDOCODE_SERVER_URL) {
    config.serverUrl = process.env.SUDOCODE_SERVER_URL;
  }

  // Fix unexpanded VS Code variables BEFORE generating project ID
  // This ensures both workingDir and projectId are consistent
  config.workingDir = fixUnexpandedVariables(config.workingDir);

  // Use project discovery to resolve projectId and sudocodeDir
  const effectiveWorkDir = config.workingDir || process.cwd();
  const sudocodeDirOverride = config.sudocodeDir || process.env.SUDOCODE_DIR;
  
  const discovery = discoverProject(effectiveWorkDir, undefined, sudocodeDirOverride);
  
  // Auto-discover project ID from working directory if not specified
  if (!config.projectId) {
    config.projectId = discovery.projectId;
    console.error(`[mcp] Discovered project: id=${discovery.projectId}, source=${discovery.source}`);
  }
  
  // Set sudocodeDir if not explicitly provided
  if (!config.sudocodeDir) {
    config.sudocodeDir = discovery.sudocodeDir;
    if (process.env.DEBUG_MCP) {
      console.error(`[mcp] Using sudocodeDir: ${discovery.sudocodeDir}`);
    }
  }
  
  // Log warning if any
  if (discovery.warning) {
    console.error(`[mcp] Warning: ${discovery.warning}`);
  }

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
