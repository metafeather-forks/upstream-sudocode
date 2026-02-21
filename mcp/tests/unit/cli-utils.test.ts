/**
 * Unit tests for CLI utility functions
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { parseArgs, generateProjectId, fixUnexpandedVariables } from "../../src/cli-utils.js";

describe("cli-utils", () => {
  describe("parseArgs", () => {
    it("should return empty config for no arguments", () => {
      const config = parseArgs(["node", "script"]);
      expect(config).toEqual({});
    });

    it("should parse --working-dir argument", () => {
      const config = parseArgs(["node", "script", "--working-dir", "/my/path"]);
      expect(config.workingDir).toBe("/my/path");
    });

    it("should parse -w shorthand for working dir", () => {
      const config = parseArgs(["node", "script", "-w", "/my/path"]);
      expect(config.workingDir).toBe("/my/path");
    });

    it("should parse --sudocode-dir argument", () => {
      const config = parseArgs(["node", "script", "--sudocode-dir", "/custom/.sudocode"]);
      expect(config.sudocodeDir).toBe("/custom/.sudocode");
    });

    it("should parse -d shorthand for sudocode dir", () => {
      const config = parseArgs(["node", "script", "-d", "/custom/.sudocode"]);
      expect(config.sudocodeDir).toBe("/custom/.sudocode");
    });

    it("should parse --db-path argument", () => {
      const config = parseArgs(["node", "script", "--db-path", "/custom/cache.db"]);
      expect(config.dbPath).toBe("/custom/cache.db");
    });

    it("should parse --db shorthand for db path", () => {
      const config = parseArgs(["node", "script", "--db", "/custom/cache.db"]);
      expect(config.dbPath).toBe("/custom/cache.db");
    });

    it("should parse --cli-path argument", () => {
      const config = parseArgs(["node", "script", "--cli-path", "/usr/local/bin/sudocode"]);
      expect(config.cliPath).toBe("/usr/local/bin/sudocode");
    });

    it("should parse --scope argument", () => {
      const config = parseArgs(["node", "script", "--scope", "default,executions"]);
      expect(config.scope).toBe("default,executions");
    });

    it("should parse -s shorthand for scope", () => {
      const config = parseArgs(["node", "script", "-s", "all"]);
      expect(config.scope).toBe("all");
    });

    it("should parse --server-url argument", () => {
      const config = parseArgs(["node", "script", "--server-url", "http://localhost:3000"]);
      expect(config.serverUrl).toBe("http://localhost:3000");
    });

    it("should parse --project-id argument", () => {
      const config = parseArgs(["node", "script", "--project-id", "my-project-abc123"]);
      expect(config.projectId).toBe("my-project-abc123");
    });

    it("should parse --no-sync argument", () => {
      const config = parseArgs(["node", "script", "--no-sync"]);
      expect(config.syncOnStartup).toBe(false);
    });

    it("should parse multiple arguments", () => {
      const config = parseArgs([
        "node", "script",
        "-w", "/my/project",
        "-d", "/custom/.sudocode",
        "--scope", "all",
        "--server-url", "http://localhost:3000"
      ]);
      expect(config.workingDir).toBe("/my/project");
      expect(config.sudocodeDir).toBe("/custom/.sudocode");
      expect(config.scope).toBe("all");
      expect(config.serverUrl).toBe("http://localhost:3000");
    });

    it("should set _showHelp for --help argument", () => {
      const config = parseArgs(["node", "script", "--help"]) as any;
      expect(config._showHelp).toBe(true);
    });

    it("should set _showHelp for -h shorthand", () => {
      const config = parseArgs(["node", "script", "-h"]) as any;
      expect(config._showHelp).toBe(true);
    });

    it("should set _unknownOption for unknown arguments", () => {
      const config = parseArgs(["node", "script", "--unknown-arg"]) as any;
      expect(config._unknownOption).toBe("--unknown-arg");
    });
  });

  describe("generateProjectId", () => {
    it("should generate ID from simple path", () => {
      const id = generateProjectId("/home/user/my-project");
      expect(id).toMatch(/^my-project-[a-f0-9]{8}$/);
    });

    it("should handle paths with special characters", () => {
      const id = generateProjectId("/home/user/My Project Name");
      expect(id).toMatch(/^my-project-name-[a-f0-9]{8}$/);
    });

    it("should truncate long repo names to 32 characters", () => {
      const longName = "this-is-a-very-long-repository-name-that-should-be-truncated";
      const id = generateProjectId(`/home/user/${longName}`);
      // The name part (before the hash) should be at most 32 chars
      const namePart = id.split("-").slice(0, -1).join("-");
      expect(namePart.length).toBeLessThanOrEqual(32);
    });

    it("should generate unique IDs for different paths with same repo name", () => {
      const id1 = generateProjectId("/home/user1/project");
      const id2 = generateProjectId("/home/user2/project");
      expect(id1).not.toBe(id2);
    });

    it("should generate consistent ID for same path", () => {
      const id1 = generateProjectId("/home/user/project");
      const id2 = generateProjectId("/home/user/project");
      expect(id1).toBe(id2);
    });

    it("should handle relative paths by resolving them", () => {
      const id1 = generateProjectId("./relative/path");
      // Should produce a valid ID format
      expect(id1).toMatch(/^[a-z0-9-]+-[a-f0-9]{8}$/);
    });

    it("should remove leading/trailing dashes from name", () => {
      const id = generateProjectId("/home/user/-project-");
      // ID should not start with a dash or have double dashes before hash
      expect(id).not.toMatch(/^-/); // no leading dash
      expect(id).not.toMatch(/--[a-f0-9]{8}$/); // no double dash before hash
      expect(id).toMatch(/^[a-z0-9-]+-[a-f0-9]{8}$/); // valid format
    });
  });

  describe("fixUnexpandedVariables", () => {
    let originalPwd: string | undefined;
    let consoleErrorSpy: any;

    beforeEach(() => {
      originalPwd = process.env.PWD;
      consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    });

    afterEach(() => {
      if (originalPwd !== undefined) {
        process.env.PWD = originalPwd;
      } else {
        delete process.env.PWD;
      }
      consoleErrorSpy.mockRestore();
    });

    it("should return undefined for undefined input", () => {
      const result = fixUnexpandedVariables(undefined);
      expect(result).toBeUndefined();
    });

    it("should return path unchanged if no variables present", () => {
      const result = fixUnexpandedVariables("/regular/path");
      expect(result).toBe("/regular/path");
    });

    it("should fix ${workspaceFolder} variable", () => {
      process.env.PWD = "/fixed/path";
      const result = fixUnexpandedVariables("${workspaceFolder}");
      expect(result).toBe("/fixed/path");
    });

    it("should fix paths containing ${...} patterns", () => {
      process.env.PWD = "/fixed/path";
      const result = fixUnexpandedVariables("${workspaceFolder}/subdir");
      expect(result).toBe("/fixed/path");
    });

    it("should log message when fixing variables", () => {
      process.env.PWD = "/fixed/path";
      fixUnexpandedVariables("${workspaceFolder}");
      expect(consoleErrorSpy).toHaveBeenCalledWith(
        expect.stringContaining("Fixed unexpanded variable")
      );
    });

    it("should fall back to cwd when PWD is not set", () => {
      delete process.env.PWD;
      const result = fixUnexpandedVariables("${workspaceFolder}");
      expect(result).toBe(process.cwd());
    });
  });
});
