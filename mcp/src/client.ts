/**
 * sudocode CLI client wrapper
 *
 * This module provides a client class that spawns `sudocode` CLI commands
 * and parses their JSON output for use in MCP tools.
 */

import { spawn } from "child_process";
import { existsSync } from "fs";
import { dirname, join } from "path";
import { fileURLToPath } from "url";
import { SudocodeClientConfig, SudocodeError } from "./types.js";
import { discoverProject } from "@sudocode-ai/cli/project-discovery";

export class SudocodeClient {
  private workingDir: string;
  private workDirExplicit: boolean;
  private cliPath: string;
  private cliArgs: string[];
  private dbPath?: string;
  private serverUrl?: string;
  private versionChecked = false;
  private sudocodeDirOverride?: string;

  constructor(config?: SudocodeClientConfig) {
    // Track if workDir was explicitly provided (via -w/--working-dir flag)
    // If explicit, we respect it and don't dynamically lookup from server
    this.workDirExplicit = !!config?.workingDir;

    // Get working directory and expand variables if needed
    let workingDir =
      config?.workingDir || process.env.SUDOCODE_WORKING_DIR || process.cwd();

    // Fix unexpanded ${workspaceFolder} variable - use PWD or cwd() instead
    if (workingDir === "${workspaceFolder}" || workingDir.includes("${")) {
      workingDir = process.env.PWD || process.cwd();
    }

    this.workingDir = workingDir;
    this.serverUrl = config?.serverUrl || process.env.SUDOCODE_SERVER_URL;
    
    // Store explicit sudocodeDir override from config (NOT from SUDOCODE_DIR env var)
    // SUDOCODE_DIR is only used as fallback when server is unavailable
    // Priority: config.sudocodeDir > server dynamic > SUDOCODE_DIR env > static fallback
    this.sudocodeDirOverride = config?.sudocodeDir;
    
    // Resolve dbPath with priority:
    // 1. Explicit config.dbPath
    // 2. SUDOCODE_DB env var
    // 3. Explicit config.sudocodeDir
    // 4. SUDOCODE_DIR env var (for project-specific databases)
    // 5. Dynamic resolution at runtime (via getDbPath()) - NOT set statically here
    // When serverUrl is configured and none of the above are set, we defer dbPath
    // resolution to avoid using stale values
    if (config?.dbPath) {
      this.dbPath = config.dbPath;
    } else if (process.env.SUDOCODE_DB) {
      this.dbPath = process.env.SUDOCODE_DB;
    } else if (this.sudocodeDirOverride) {
      // Explicit config.sudocodeDir was provided
      this.dbPath = join(this.sudocodeDirOverride, "cache.db");
    } else if (process.env.SUDOCODE_DIR) {
      // SUDOCODE_DIR env var provides project-specific directory
      // This is typically set by direnv or the shell to point to the correct
      // project database (e.g., .sudocode/projects/<project-id>/cache.db)
      this.dbPath = join(process.env.SUDOCODE_DIR, "cache.db");
    }
    // Note: When serverUrl is set and no explicit dbPath/sudocodeDir, we DON'T
    // set this.dbPath. This allows getSudocodeDir() to dynamically resolve and
    // the CLI to use its own resolution based on the working directory

    // Auto-discover CLI path from node_modules or use configured/env path
    const cliInfo = this.findCliPath();
    this.cliPath = cliInfo.path;
    this.cliArgs = cliInfo.args;
  }

  /**
   * Find the CLI by looking in node_modules/@sudocode-ai/cli
   * Since we added @sudocode-ai/cli as a dependency, it should be there
   */
  private findCliPath(): { path: string; args: string[] } {
    try {
      const currentFile = fileURLToPath(import.meta.url);
      const currentDir = dirname(currentFile);

      // Look for @sudocode-ai/cli in various possible locations
      const possiblePaths = [
        // Workspace root node_modules (development)
        join(
          currentDir,
          "..",
          "..",
          "node_modules",
          "@sudocode-ai",
          "cli",
          "dist",
          "cli.js"
        ),
        // Local package node_modules (when installed from npm)
        join(
          currentDir,
          "..",
          "node_modules",
          "@sudocode-ai",
          "cli",
          "dist",
          "cli.js"
        ),
      ];

      for (const cliJsPath of possiblePaths) {
        if (existsSync(cliJsPath)) {
          // Return node + cli.js path instead of creating a wrapper
          return {
            path: process.execPath, // Use current node binary
            args: [cliJsPath], // Pass cli.js as first argument
          };
        }
      }
    } catch (error) {
      // Ignore errors and fall back to 'sudocode' command
    }

    // Fall back to 'sudocode' command in PATH
    return { path: "sudocode", args: [] };
  }

  /**
   * Get the active working directory from the sudocode server.
   * 
   * This queries the server for the UI's currently selected project and returns
   * its path. Falls back to the cached workingDir if the server is unavailable
   * or no project is selected.
   * 
   * This enables the MCP server to correctly target the project the user
   * is currently working on in the UI, even if it changed after MCP startup.
   * 
   * Note: If workDir was explicitly provided via -w/--working-dir flag,
   * this returns the explicit path without querying the server.
   */
  async getActiveWorkDir(): Promise<string> {
    // If workDir was explicitly provided, respect it
    if (this.workDirExplicit) {
      return this.workingDir;
    }

    if (!this.serverUrl) {
      return this.workingDir;
    }

    try {
      const response = await fetch(`${this.serverUrl}/api/projects/open`, {
        method: "GET",
        headers: {
          "Accept": "application/json",
        },
        signal: AbortSignal.timeout(2000), // Quick timeout - don't block CLI ops
      });

      if (!response.ok) {
        // Server returned an error - fall back to cached workingDir
        return this.workingDir;
      }

      const result = await response.json() as {
        success: boolean;
        currentProjectId?: string;
        data: Array<{
          id: string;
          path: string;
          name: string;
          isCurrent?: boolean;
          openedAt?: string;
        }>;
      };

      if (result.success && result.data && result.data.length > 0) {
        // Find the UI's current project (marked with isCurrent or first in sorted list)
        const currentProject = result.data.find(p => p.isCurrent) || result.data[0];
        if (currentProject.path && currentProject.path !== this.workingDir) {
          console.error(
            `[SudocodeClient] Using UI's current project path: ${currentProject.path} (was: ${this.workingDir})`
          );
          return currentProject.path;
        }
        return currentProject.path || this.workingDir;
      }
    } catch (error) {
      // Server not available or timeout - fall back silently
      // This is expected when running without the local server
      if (process.env.DEBUG_MCP) {
        console.error(
          `[SudocodeClient] Could not get active project from server: ${error instanceof Error ? error.message : error}`
        );
      }
    }

    return this.workingDir;
  }

  /**
   * Get the sudocode directory with dynamic resolution based on working directory.
   * 
   * Resolution priority:
   * 1. Explicit config override (config.sudocodeDir)
   * 2. SUDOCODE_DIR env var
   * 3. Project discovery from registry (offline-first, reads ~/.config/sudocode/projects.json)
   * 4. Fallback: join(workingDir, ".sudocode")
   * 
   * IMPORTANT: This resolves based on the working directory using local registry lookup,
   * NOT server queries. This is faster and works offline.
   */
  getSudocodeDir(): string {
    // 1. Explicit config override takes highest precedence
    if (this.sudocodeDirOverride) {
      return this.sudocodeDirOverride;
    }

    // 2. SUDOCODE_DIR env var 
    if (process.env.SUDOCODE_DIR) {
      if (process.env.DEBUG_MCP) {
        console.error(
          `[SudocodeClient] Using SUDOCODE_DIR: ${process.env.SUDOCODE_DIR}`
        );
      }
      return process.env.SUDOCODE_DIR;
    }

    // 3. Use CLI's project discovery (reads local registry file)
    const discovery = discoverProject(this.workingDir);
    if (process.env.DEBUG_MCP) {
      console.error(
        `[SudocodeClient] Project discovery for ${this.workingDir}: source=${discovery.source}, sudocodeDir=${discovery.sudocodeDir}`
      );
    }
    return discovery.sudocodeDir;
  }

  /**
   * Execute a CLI command and return parsed JSON output
   */
  async exec(args: string[], options?: { timeout?: number }): Promise<any> {
    // Check CLI version on first call
    if (!this.versionChecked) {
      await this.checkVersion();
      this.versionChecked = true;
    }

    // Build command arguments - prepend cliArgs (e.g., cli.js path)
    const cmdArgs = [...this.cliArgs, ...args];

    // Add --json flag if not already present
    if (!cmdArgs.includes("--json")) {
      cmdArgs.push("--json");
    }

    // Add --db flag - use static dbPath or dynamically resolve from getSudocodeDir()
    // This ensures the correct project-specific database is used even when the
    // MCP server is shared across multiple projects
    if (!cmdArgs.includes("--db")) {
      if (this.dbPath) {
        // Use statically configured dbPath
        cmdArgs.push("--db", this.dbPath);
      } else {
        // Dynamically resolve database path from current project's sudocodeDir
        const sudocodeDir = this.getSudocodeDir();
        const dynamicDbPath = join(sudocodeDir, "cache.db");
        cmdArgs.push("--db", dynamicDbPath);
        if (process.env.DEBUG_MCP) {
          console.error(`[SudocodeClient] Using dynamic dbPath: ${dynamicDbPath}`);
        }
      }
    }

    // Get the active working directory (may query server if configured)
    const workDir = await this.getActiveWorkDir();

    return new Promise((resolve, reject) => {
      const proc = spawn(this.cliPath, cmdArgs, {
        cwd: workDir,
        env: {
          ...process.env,
          SUDOCODE_DISABLE_UPDATE_CHECK: "true",
          ...(process.env.SUDOCODE_SESSION_ID
            ? { SUDOCODE_SESSION_ID: process.env.SUDOCODE_SESSION_ID }
            : {}),
        },
      });

      let stdout = "";
      let stderr = "";

      proc.stdout.on("data", (data) => {
        stdout += data.toString();
      });

      proc.stderr.on("data", (data) => {
        stderr += data.toString();
      });

      // Set timeout if specified
      const timeout = options?.timeout || 30000; // Default 30s
      const timer = setTimeout(() => {
        proc.kill();
        reject(
          new SudocodeError(
            `Command timed out after ${timeout}ms`,
            -1,
            "Timeout"
          )
        );
      }, timeout);

      proc.on("close", (code) => {
        clearTimeout(timer);

        if (code !== 0) {
          reject(
            new SudocodeError(
              `CLI command failed with exit code ${code}`,
              code || -1,
              stderr
            )
          );
          return;
        }

        // Parse JSON output
        try {
          const result = JSON.parse(stdout);
          resolve(result);
        } catch (error) {
          reject(
            new SudocodeError(
              `Failed to parse JSON output: ${
                error instanceof Error ? error.message : String(error)
              }`,
              -1,
              stdout
            )
          );
        }
      });

      proc.on("error", (error) => {
        clearTimeout(timer);
        reject(
          new SudocodeError(
            `Failed to spawn CLI: ${error.message}`,
            -1,
            error.message
          )
        );
      });
    });
  }

  /**
   * Check that the CLI is installed and get its version
   */
  async checkVersion(): Promise<{ version: string }> {
    try {
      const proc = spawn(this.cliPath, [...this.cliArgs, "--version"], {
        cwd: this.workingDir,
        env: {
          ...process.env,
          SUDOCODE_DISABLE_UPDATE_CHECK: "true",
        },
      });

      let stdout = "";
      let stderr = "";

      proc.stdout.on("data", (data) => {
        stdout += data.toString();
      });

      proc.stderr.on("data", (data) => {
        stderr += data.toString();
      });

      return new Promise((resolve, reject) => {
        proc.on("close", (code) => {
          if (code !== 0) {
            reject(
              new SudocodeError(
                `CLI not found or failed to execute. Make sure 'sudocode' is installed and in your PATH.`,
                code || -1,
                stderr
              )
            );
            return;
          }

          // Version output format: "sudocode version X.Y.Z" or just "X.Y.Z"
          const versionMatch = stdout.match(/(\d+\.\d+\.\d+)/);
          const version = versionMatch ? versionMatch[1] : stdout.trim();

          resolve({ version });
        });

        proc.on("error", () => {
          reject(
            new SudocodeError(
              `CLI not found at path: ${this.cliPath}. Make sure 'sudocode' is installed.`,
              -1,
              "CLI not found"
            )
          );
        });
      });
    } catch (error) {
      throw new SudocodeError(
        `Failed to check CLI version: ${
          error instanceof Error ? error.message : String(error)
        }`,
        -1,
        ""
      );
    }
  }
}
