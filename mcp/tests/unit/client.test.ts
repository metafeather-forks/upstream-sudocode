/**
 * Unit tests for SudocodeClient
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { spawn } from "child_process";
import { EventEmitter } from "events";
import { SudocodeClient } from "../../src/client.js";
import { SudocodeError } from "../../src/types.js";

// Mock child_process
vi.mock("child_process");

describe("SudocodeClient", () => {
  let mockSpawn: any;
  let mockProcess: any;

  beforeEach(() => {
    // Create mock process
    mockProcess = new EventEmitter();
    mockProcess.stdout = new EventEmitter();
    mockProcess.stderr = new EventEmitter();
    mockProcess.kill = vi.fn();

    // Mock spawn to return our mock process
    mockSpawn = vi.mocked(spawn);
    mockSpawn.mockReturnValue(mockProcess as any);
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  describe("constructor", () => {
    it("should use default configuration", () => {
      const client = new SudocodeClient();
      expect(client).toBeDefined();
    });

    it("should use provided configuration", () => {
      const config = {
        workingDir: "/custom/path",
        cliPath: "/usr/local/bin/sg",
        dbPath: "/custom/cache.db",
      };
      const client = new SudocodeClient(config);
      expect(client).toBeDefined();
    });

    it("should read from environment variables", () => {
      process.env.SUDOCODE_WORKING_DIR = "/env/path";
      process.env.SUDOCODE_PATH = "sudocode-custom";
      process.env.SUDOCODE_DB = "/env/cache.db";

      const client = new SudocodeClient();
      expect(client).toBeDefined();

      // Cleanup
      delete process.env.SUDOCODE_WORKING_DIR;
      delete process.env.SUDOCODE_PATH;
      delete process.env.SUDOCODE_DB;
    });
  });

  describe("exec", () => {
    it("should spawn CLI with correct arguments", async () => {
      const client = new SudocodeClient();

      // Mock version check
      const versionProcess = new EventEmitter() as any;
      versionProcess.stdout = new EventEmitter();
      versionProcess.stderr = new EventEmitter();
      mockSpawn.mockReturnValueOnce(versionProcess);

      // Emit version response
      setImmediate(() => {
        versionProcess.stdout.emit("data", "0.1.0\n");
        versionProcess.emit("close", 0);
      });

      // Mock actual command
      mockSpawn.mockReturnValueOnce(mockProcess);
      setImmediate(() => {
        mockProcess.stdout.emit("data", '{"result": "success"}');
        mockProcess.emit("close", 0);
      });

      const result = await client.exec(["issue", "list"]);

      expect(mockSpawn).toHaveBeenCalledTimes(2); // version + command

      // When CLI is found in node_modules, it uses node binary + cli.js
      // Otherwise it uses "sudocode" command
      const lastCall = mockSpawn.mock.calls[1];
      const [command, args, options] = lastCall;

      // Check that the command includes the correct arguments
      if (command === "sudocode") {
        // CLI found in PATH
        expect(args).toEqual(
          expect.arrayContaining(["issue", "list", "--json"])
        );
      } else {
        // CLI found in node_modules - uses node binary
        expect(command).toBe(process.execPath);
        expect(args).toEqual(
          expect.arrayContaining(["issue", "list", "--json"])
        );
        expect(args[0]).toContain("cli.js");
      }

      expect(options).toEqual(
        expect.objectContaining({
          cwd: expect.any(String),
          env: expect.objectContaining({
            SUDOCODE_DISABLE_UPDATE_CHECK: "true",
          }),
        })
      );
      expect(result).toEqual({ result: "success" });
    });

    it("should automatically add --json flag", async () => {
      const client = new SudocodeClient();

      // Mock version check
      const versionProcess = new EventEmitter() as any;
      versionProcess.stdout = new EventEmitter();
      versionProcess.stderr = new EventEmitter();
      mockSpawn.mockReturnValueOnce(versionProcess);
      setImmediate(() => {
        versionProcess.stdout.emit("data", "0.1.0\n");
        versionProcess.emit("close", 0);
      });

      // Mock actual command
      mockSpawn.mockReturnValueOnce(mockProcess);
      setImmediate(() => {
        mockProcess.stdout.emit("data", "[]");
        mockProcess.emit("close", 0);
      });

      await client.exec(["ready"]);

      const lastCall = mockSpawn.mock.calls[mockSpawn.mock.calls.length - 1];
      expect(lastCall[1]).toContain("--json");
    });

    it("should not duplicate --json flag if already present", async () => {
      const client = new SudocodeClient();

      // Mock version check
      const versionProcess = new EventEmitter() as any;
      versionProcess.stdout = new EventEmitter();
      versionProcess.stderr = new EventEmitter();
      mockSpawn.mockReturnValueOnce(versionProcess);
      setImmediate(() => {
        versionProcess.stdout.emit("data", "0.1.0\n");
        versionProcess.emit("close", 0);
      });

      // Mock actual command
      mockSpawn.mockReturnValueOnce(mockProcess);
      setImmediate(() => {
        mockProcess.stdout.emit("data", "[]");
        mockProcess.emit("close", 0);
      });

      await client.exec(["ready", "--json"]);

      const lastCall = mockSpawn.mock.calls[mockSpawn.mock.calls.length - 1];
      const jsonCount = lastCall[1].filter(
        (arg: string) => arg === "--json"
      ).length;
      expect(jsonCount).toBe(1);
    });

    it("should add --db flag when dbPath is configured", async () => {
      const client = new SudocodeClient({ dbPath: "/custom/cache.db" });

      // Mock version check
      const versionProcess = new EventEmitter() as any;
      versionProcess.stdout = new EventEmitter();
      versionProcess.stderr = new EventEmitter();
      mockSpawn.mockReturnValueOnce(versionProcess);
      setImmediate(() => {
        versionProcess.stdout.emit("data", "0.1.0\n");
        versionProcess.emit("close", 0);
      });

      // Mock actual command
      mockSpawn.mockReturnValueOnce(mockProcess);
      setImmediate(() => {
        mockProcess.stdout.emit("data", "{}");
        mockProcess.emit("close", 0);
      });

      await client.exec(["stats"]);

      const lastCall = mockSpawn.mock.calls[mockSpawn.mock.calls.length - 1];
      expect(lastCall[1]).toContain("--db");
      expect(lastCall[1]).toContain("/custom/cache.db");
    });

    it("should set SUDOCODE_DISABLE_UPDATE_CHECK environment variable", async () => {
      const client = new SudocodeClient();

      // Mock version check
      const versionProcess = new EventEmitter() as any;
      versionProcess.stdout = new EventEmitter();
      versionProcess.stderr = new EventEmitter();
      mockSpawn.mockReturnValueOnce(versionProcess);
      setImmediate(() => {
        versionProcess.stdout.emit("data", "0.1.0\n");
        versionProcess.emit("close", 0);
      });

      // Mock actual command
      mockSpawn.mockReturnValueOnce(mockProcess);
      setImmediate(() => {
        mockProcess.stdout.emit("data", "{}");
        mockProcess.emit("close", 0);
      });

      await client.exec(["stats"]);

      const lastCall = mockSpawn.mock.calls[mockSpawn.mock.calls.length - 1];
      const [_, __, options] = lastCall;
      expect(options.env.SUDOCODE_DISABLE_UPDATE_CHECK).toBe("true");
    });

    it("should parse JSON output correctly", async () => {
      const client = new SudocodeClient();

      // Mock version check
      const versionProcess = new EventEmitter() as any;
      versionProcess.stdout = new EventEmitter();
      versionProcess.stderr = new EventEmitter();
      mockSpawn.mockReturnValueOnce(versionProcess);
      setImmediate(() => {
        versionProcess.stdout.emit("data", "0.1.0\n");
        versionProcess.emit("close", 0);
      });

      // Mock actual command
      mockSpawn.mockReturnValueOnce(mockProcess);
      const testData = { issues: [{ id: "ISSUE-001", title: "Test" }] };
      setImmediate(() => {
        mockProcess.stdout.emit("data", JSON.stringify(testData));
        mockProcess.emit("close", 0);
      });

      const result = await client.exec(["issue", "list"]);
      expect(result).toEqual(testData);
    });

    it("should handle multi-chunk stdout", async () => {
      const client = new SudocodeClient();

      // Mock version check
      const versionProcess = new EventEmitter() as any;
      versionProcess.stdout = new EventEmitter();
      versionProcess.stderr = new EventEmitter();
      mockSpawn.mockReturnValueOnce(versionProcess);
      setImmediate(() => {
        versionProcess.stdout.emit("data", "0.1.0\n");
        versionProcess.emit("close", 0);
      });

      // Mock actual command with chunked output
      mockSpawn.mockReturnValueOnce(mockProcess);
      setImmediate(() => {
        mockProcess.stdout.emit("data", '{"result":');
        mockProcess.stdout.emit("data", ' "success"}');
        mockProcess.emit("close", 0);
      });

      const result = await client.exec(["stats"]);
      expect(result).toEqual({ result: "success" });
    });

    it("should throw SudocodeError on non-zero exit code", async () => {
      const client = new SudocodeClient();

      // Mock version check
      const versionProcess = new EventEmitter() as any;
      versionProcess.stdout = new EventEmitter();
      versionProcess.stderr = new EventEmitter();
      mockSpawn.mockReturnValueOnce(versionProcess);
      setImmediate(() => {
        versionProcess.stdout.emit("data", "0.1.0\n");
        versionProcess.emit("close", 0);
      });

      // Mock failing command
      mockSpawn.mockReturnValueOnce(mockProcess);
      setImmediate(() => {
        mockProcess.stderr.emit("data", "Error: Issue not found\n");
        mockProcess.emit("close", 1);
      });

      await expect(client.exec(["issue", "show", "ISSUE-999"])).rejects.toThrow(
        SudocodeError
      );
    });

    it("should include stderr in error message", async () => {
      const client = new SudocodeClient();

      // Mock version check
      const versionProcess = new EventEmitter() as any;
      versionProcess.stdout = new EventEmitter();
      versionProcess.stderr = new EventEmitter();
      mockSpawn.mockReturnValueOnce(versionProcess);
      setImmediate(() => {
        versionProcess.stdout.emit("data", "0.1.0\n");
        versionProcess.emit("close", 0);
      });

      // Mock failing command
      mockSpawn.mockReturnValueOnce(mockProcess);
      const errorMessage = "Error: Database not found";
      setImmediate(() => {
        mockProcess.stderr.emit("data", errorMessage);
        mockProcess.emit("close", 1);
      });

      try {
        await client.exec(["stats"]);
        expect.fail("Should have thrown an error");
      } catch (error) {
        expect(error).toBeInstanceOf(SudocodeError);
        expect((error as SudocodeError).stderr).toBe(errorMessage);
        expect((error as SudocodeError).exitCode).toBe(1);
      }
    });

    it("should throw error on malformed JSON", async () => {
      const client = new SudocodeClient();

      // Mock version check
      const versionProcess = new EventEmitter() as any;
      versionProcess.stdout = new EventEmitter();
      versionProcess.stderr = new EventEmitter();
      mockSpawn.mockReturnValueOnce(versionProcess);
      setImmediate(() => {
        versionProcess.stdout.emit("data", "0.1.0\n");
        versionProcess.emit("close", 0);
      });

      // Mock command with invalid JSON
      mockSpawn.mockReturnValueOnce(mockProcess);
      setImmediate(() => {
        mockProcess.stdout.emit("data", "{invalid json}");
        mockProcess.emit("close", 0);
      });

      await expect(client.exec(["stats"])).rejects.toThrow(SudocodeError);
    });

    it("should timeout after specified duration", async () => {
      const client = new SudocodeClient();

      // Mock version check
      const versionProcess = new EventEmitter() as any;
      versionProcess.stdout = new EventEmitter();
      versionProcess.stderr = new EventEmitter();
      mockSpawn.mockReturnValueOnce(versionProcess);
      setImmediate(() => {
        versionProcess.stdout.emit("data", "0.1.0\n");
        versionProcess.emit("close", 0);
      });

      // Mock command that never completes
      mockSpawn.mockReturnValueOnce(mockProcess);
      // Don't emit close event

      await expect(client.exec(["stats"], { timeout: 100 })).rejects.toThrow(
        "timed out"
      );
      expect(mockProcess.kill).toHaveBeenCalled();
    }, 1000);

    it("should handle spawn errors", async () => {
      const client = new SudocodeClient({ cliPath: "/nonexistent/sg" });

      // Mock version check failure
      const versionProcess = new EventEmitter() as any;
      versionProcess.stdout = new EventEmitter();
      versionProcess.stderr = new EventEmitter();
      mockSpawn.mockReturnValueOnce(versionProcess);
      setImmediate(() => {
        versionProcess.emit("error", new Error("ENOENT"));
      });

      await expect(client.exec(["stats"])).rejects.toThrow(SudocodeError);
    });
  });

  describe("checkVersion", () => {
    it("should successfully check CLI version", async () => {
      const client = new SudocodeClient();

      mockSpawn.mockReturnValueOnce(mockProcess);
      setImmediate(() => {
        mockProcess.stdout.emit("data", "sg version 0.1.0\n");
        mockProcess.emit("close", 0);
      });

      const result = await client.checkVersion();
      expect(result).toEqual({ version: "0.1.0" });
    });

    it("should parse version from numeric output", async () => {
      const client = new SudocodeClient();

      mockSpawn.mockReturnValueOnce(mockProcess);
      setImmediate(() => {
        mockProcess.stdout.emit("data", "1.2.3\n");
        mockProcess.emit("close", 0);
      });

      const result = await client.checkVersion();
      expect(result).toEqual({ version: "1.2.3" });
    });

    it("should throw error when CLI not found", async () => {
      const client = new SudocodeClient({ cliPath: "/nonexistent" });

      mockSpawn.mockReturnValueOnce(mockProcess);

      const promise = client.checkVersion();

      // Emit error immediately
      setImmediate(() => {
        mockProcess.emit("error", new Error("ENOENT"));
      });

      await expect(promise).rejects.toThrow(SudocodeError);
      await expect(promise).rejects.toThrow("CLI not found");
    });

    it("should throw error on non-zero exit code", async () => {
      const client = new SudocodeClient();

      mockSpawn.mockReturnValueOnce(mockProcess);
      setImmediate(() => {
        mockProcess.stderr.emit("data", "Command not found\n");
        mockProcess.emit("close", 127);
      });

      await expect(client.checkVersion()).rejects.toThrow(SudocodeError);
    });

    it("should set SUDOCODE_DISABLE_UPDATE_CHECK when checking version", async () => {
      const client = new SudocodeClient();

      mockSpawn.mockReturnValueOnce(mockProcess);
      setImmediate(() => {
        mockProcess.stdout.emit("data", "0.1.0\n");
        mockProcess.emit("close", 0);
      });

      await client.checkVersion();

      const lastCall = mockSpawn.mock.calls[mockSpawn.mock.calls.length - 1];
      const [_, __, options] = lastCall;
      expect(options.env.SUDOCODE_DISABLE_UPDATE_CHECK).toBe("true");
    });
  });

  describe("getActiveWorkDir", () => {
    let originalFetch: typeof global.fetch;

    beforeEach(() => {
      originalFetch = global.fetch;
    });

    afterEach(() => {
      global.fetch = originalFetch;
      delete process.env.SUDOCODE_SERVER_URL;
    });

    it("should return explicit workingDir when provided via config", async () => {
      const client = new SudocodeClient({ workingDir: "/explicit/path" });
      expect(await client.getActiveWorkDir()).toBe("/explicit/path");
    });

    it("should not query server when workingDir is explicitly provided", async () => {
      const mockFetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({
          success: true,
          data: [
            {
              id: "proj-1",
              path: "/server/path",
              name: "Test Project",
              isCurrent: true,
            },
          ],
        }),
      });
      global.fetch = mockFetch;

      const client = new SudocodeClient({
        workingDir: "/explicit/path",
        serverUrl: "http://localhost:3002",
      });
      const result = await client.getActiveWorkDir();
      expect(result).toBe("/explicit/path");
      // fetch should not be called when workDir was explicitly provided
      expect(mockFetch).not.toHaveBeenCalled();
    });

    it("should query server for current project path when no explicit workDir", async () => {
      const mockFetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({
          success: true,
          data: [
            {
              id: "proj-1",
              path: "/server/project/path",
              name: "Test Project",
              isCurrent: true,
            },
          ],
        }),
      });
      global.fetch = mockFetch;

      const client = new SudocodeClient({
        serverUrl: "http://localhost:3002",
      });
      const result = await client.getActiveWorkDir();
      expect(result).toBe("/server/project/path");
      expect(mockFetch).toHaveBeenCalledWith(
        "http://localhost:3002/api/projects/open",
        expect.objectContaining({
          method: "GET",
          headers: { Accept: "application/json" },
        })
      );
    });

    it("should fall back to cached workingDir when server unavailable", async () => {
      const mockFetch = vi.fn().mockRejectedValue(new Error("Network error"));
      global.fetch = mockFetch;

      const client = new SudocodeClient({
        serverUrl: "http://localhost:3002",
      });
      const result = await client.getActiveWorkDir();
      // Should fall back to process.cwd() since no explicit workDir
      expect(result).toBe(process.cwd());
    });

    it("should fall back when server returns non-ok response", async () => {
      const mockFetch = vi.fn().mockResolvedValue({
        ok: false,
        status: 500,
      });
      global.fetch = mockFetch;

      const client = new SudocodeClient({
        serverUrl: "http://localhost:3002",
      });
      const result = await client.getActiveWorkDir();
      expect(result).toBe(process.cwd());
    });

    it("should return first project when none marked as current", async () => {
      const mockFetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({
          success: true,
          data: [
            {
              id: "proj-1",
              path: "/first/project",
              name: "First Project",
            },
            {
              id: "proj-2",
              path: "/second/project",
              name: "Second Project",
            },
          ],
        }),
      });
      global.fetch = mockFetch;

      const client = new SudocodeClient({
        serverUrl: "http://localhost:3002",
      });
      const result = await client.getActiveWorkDir();
      expect(result).toBe("/first/project");
    });

    it("should prefer project marked as isCurrent", async () => {
      const mockFetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({
          success: true,
          data: [
            {
              id: "proj-1",
              path: "/first/project",
              name: "First Project",
              isCurrent: false,
            },
            {
              id: "proj-2",
              path: "/current/project",
              name: "Current Project",
              isCurrent: true,
            },
          ],
        }),
      });
      global.fetch = mockFetch;

      const client = new SudocodeClient({
        serverUrl: "http://localhost:3002",
      });
      const result = await client.getActiveWorkDir();
      expect(result).toBe("/current/project");
    });

    it("should fall back when server returns no serverUrl configured", async () => {
      const client = new SudocodeClient();
      const result = await client.getActiveWorkDir();
      expect(result).toBe(process.cwd());
    });

    it("should fall back when server returns empty data array", async () => {
      const mockFetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({
          success: true,
          data: [],
        }),
      });
      global.fetch = mockFetch;

      const client = new SudocodeClient({
        serverUrl: "http://localhost:3002",
      });
      const result = await client.getActiveWorkDir();
      expect(result).toBe(process.cwd());
    });
  });

  describe("getSudocodeDir", () => {
    let originalFetch: typeof global.fetch;

    beforeEach(() => {
      originalFetch = global.fetch;
    });

    afterEach(() => {
      global.fetch = originalFetch;
      delete process.env.SUDOCODE_DIR;
      delete process.env.SUDOCODE_SERVER_URL;
    });

    it("should return explicit override when provided via config", async () => {
      const client = new SudocodeClient({ sudocodeDir: "/custom/.sudocode" });
      expect(await client.getSudocodeDir()).toBe("/custom/.sudocode");
    });

    it("should return SUDOCODE_DIR env var when no server configured (fallback)", async () => {
      process.env.SUDOCODE_DIR = "/env/.sudocode";
      // No serverUrl configured - SUDOCODE_DIR acts as fallback
      const client = new SudocodeClient();
      expect(await client.getSudocodeDir()).toBe("/env/.sudocode");
    });

    it("should use local registry discovery for sudocodeDir by workingDir path", () => {
      // Note: getSudocodeDir now uses local registry (discoverProject) instead of server queries
      // Without a registry file, it falls back to <workingDir>/.sudocode
      const client = new SudocodeClient({
        serverUrl: "http://localhost:3002",
        workingDir: "/project/path",
      });
      const result = client.getSudocodeDir();
      // Falls back to workingDir/.sudocode when no registry entry exists
      expect(result).toBe("/project/path/.sudocode");
    });

    it("should fall back to static when no registry entry exists", () => {
      const client = new SudocodeClient({
        workingDir: "/my/project",
        serverUrl: "http://localhost:3002",
      });
      const result = client.getSudocodeDir();
      expect(result).toBe("/my/project/.sudocode");
    });

    it("should fall back to static when no matching entry in registry", () => {
      const client = new SudocodeClient({
        workingDir: "/my/project",
        serverUrl: "http://localhost:3002",
      });
      const result = client.getSudocodeDir();
      expect(result).toBe("/my/project/.sudocode");
    });

    it("should fall back to static when serverUrl is configured but no registry entry", () => {
      const client = new SudocodeClient({
        workingDir: "/my/project",
        serverUrl: "http://localhost:3002",
      });
      const result = client.getSudocodeDir();
      expect(result).toBe("/my/project/.sudocode");
    });

    it("should fall back to static when no server URL configured", () => {
      const client = new SudocodeClient({
        workingDir: "/my/project",
      });
      const result = client.getSudocodeDir();
      expect(result).toBe("/my/project/.sudocode");
    });

    it("should prioritize config override over env var", () => {
      process.env.SUDOCODE_DIR = "/env/.sudocode";
      const client = new SudocodeClient({ sudocodeDir: "/config/.sudocode" });
      expect(client.getSudocodeDir()).toBe("/config/.sudocode");
    });

    it("should use SUDOCODE_DIR env var when set", () => {
      // In the new implementation, SUDOCODE_DIR takes precedence over registry discovery
      process.env.SUDOCODE_DIR = "/env/.sudocode";
      const client = new SudocodeClient({
        serverUrl: "http://localhost:3002",
        workingDir: "/project/path",
      });
      const result = client.getSudocodeDir();
      // SUDOCODE_DIR is used directly when set
      expect(result).toBe("/env/.sudocode");
    });

    it("should use SUDOCODE_DIR when set", () => {
      process.env.SUDOCODE_DIR = "/env/.sudocode";
      const client = new SudocodeClient({
        serverUrl: "http://localhost:3002",
        workingDir: "/my/project",
      });
      const result = client.getSudocodeDir();
      // SUDOCODE_DIR is returned when set
      expect(result).toBe("/env/.sudocode");
    });

    it("should fall back to static path for nested workingDir when no registry entry", () => {
      // Note: Without a registry file, discoverProject falls back to <workingDir>/.sudocode
      // To test ancestor matching, we would need to mock the registry file
      const client = new SudocodeClient({
        serverUrl: "http://localhost:3002",
        workingDir: "/project-root/src/components",
      });

      const result = client.getSudocodeDir();
      // Falls back to workingDir/.sudocode without registry
      expect(result).toBe("/project-root/src/components/.sudocode");
    });

    it("should return static path immediately (no network calls)", () => {
      // getSudocodeDir is now synchronous and doesn't make network calls
      const client = new SudocodeClient({
        serverUrl: "http://localhost:3002",
        workingDir: "/my/project",
      });

      const result = client.getSudocodeDir();
      
      // Should fall back to static path when no registry entry
      expect(result).toBe("/my/project/.sudocode");
    });

    it("should use registry discovery (no longer depends on isCurrent)", () => {
      // Note: The new implementation uses local registry file, not server
      // Without a registry entry, it falls back to workingDir/.sudocode
      const client = new SudocodeClient({
        serverUrl: "http://localhost:3002",
        workingDir: "/project-1",
      });

      const result = client.getSudocodeDir();
      
      // Falls back to static path without registry entry
      expect(result).toBe("/project-1/.sudocode");
    });

    it("should fall back to static path when no matching project found", () => {
      // workingDir doesn't match any registered project (no registry in test)
      const client = new SudocodeClient({
        serverUrl: "http://localhost:3002",
        workingDir: "/unregistered/project",
      });

      const result = client.getSudocodeDir();
      
      // Should fall back to static <workingDir>/.sudocode
      expect(result).toBe("/unregistered/project/.sudocode");
    });

    it("should fall back to static path when no registry entry for project path", () => {
      // Without a registry entry, falls back to workingDir/.sudocode
      const client = new SudocodeClient({
        serverUrl: "http://localhost:3002",
        workingDir: "/Users/dev/my-project",
      });

      const result = client.getSudocodeDir();
      
      // Falls back to workingDir/.sudocode
      expect(result).toBe("/Users/dev/my-project/.sudocode");
    });
  });

  describe("dbPath resolution", () => {
    afterEach(() => {
      delete process.env.SUDOCODE_DIR;
      delete process.env.SUDOCODE_DB;
    });

    it("should set dbPath from SUDOCODE_DIR env var for project-specific databases", async () => {
      // SUDOCODE_DIR env var provides the project-specific database location
      // This is typically set by direnv or shell to point to the correct project
      process.env.SUDOCODE_DIR = "/custom/.sudocode";
      const client = new SudocodeClient();

      // Mock version check
      const versionProcess = new EventEmitter() as any;
      versionProcess.stdout = new EventEmitter();
      versionProcess.stderr = new EventEmitter();
      mockSpawn.mockReturnValueOnce(versionProcess);
      setImmediate(() => {
        versionProcess.stdout.emit("data", "0.1.0\n");
        versionProcess.emit("close", 0);
      });

      // Mock actual command
      mockSpawn.mockReturnValueOnce(mockProcess);
      setImmediate(() => {
        mockProcess.stdout.emit("data", "{}");
        mockProcess.emit("close", 0);
      });

      await client.exec(["stats"]);

      const lastCall = mockSpawn.mock.calls[mockSpawn.mock.calls.length - 1];
      // dbPath should be set from SUDOCODE_DIR
      expect(lastCall[1]).toContain("--db");
      expect(lastCall[1]).toContain("/custom/.sudocode/cache.db");
    });

    it("should default to <sudocodeDir>/cache.db when config.sudocodeDir is set", async () => {
      const client = new SudocodeClient({ sudocodeDir: "/config/.sudocode" });

      // Mock version check
      const versionProcess = new EventEmitter() as any;
      versionProcess.stdout = new EventEmitter();
      versionProcess.stderr = new EventEmitter();
      mockSpawn.mockReturnValueOnce(versionProcess);
      setImmediate(() => {
        versionProcess.stdout.emit("data", "0.1.0\n");
        versionProcess.emit("close", 0);
      });

      // Mock actual command
      mockSpawn.mockReturnValueOnce(mockProcess);
      setImmediate(() => {
        mockProcess.stdout.emit("data", "{}");
        mockProcess.emit("close", 0);
      });

      await client.exec(["stats"]);

      const lastCall = mockSpawn.mock.calls[mockSpawn.mock.calls.length - 1];
      expect(lastCall[1]).toContain("--db");
      expect(lastCall[1]).toContain("/config/.sudocode/cache.db");
    });

    it("should prioritize explicit dbPath over sudocodeDir", async () => {
      process.env.SUDOCODE_DIR = "/env/.sudocode";
      const client = new SudocodeClient({ dbPath: "/explicit/cache.db" });

      // Mock version check
      const versionProcess = new EventEmitter() as any;
      versionProcess.stdout = new EventEmitter();
      versionProcess.stderr = new EventEmitter();
      mockSpawn.mockReturnValueOnce(versionProcess);
      setImmediate(() => {
        versionProcess.stdout.emit("data", "0.1.0\n");
        versionProcess.emit("close", 0);
      });

      // Mock actual command
      mockSpawn.mockReturnValueOnce(mockProcess);
      setImmediate(() => {
        mockProcess.stdout.emit("data", "{}");
        mockProcess.emit("close", 0);
      });

      await client.exec(["stats"]);

      const lastCall = mockSpawn.mock.calls[mockSpawn.mock.calls.length - 1];
      expect(lastCall[1]).toContain("--db");
      expect(lastCall[1]).toContain("/explicit/cache.db");
    });

    it("should prioritize SUDOCODE_DB env var over SUDOCODE_DIR", async () => {
      process.env.SUDOCODE_DIR = "/env/.sudocode";
      process.env.SUDOCODE_DB = "/env/custom.db";
      const client = new SudocodeClient();

      // Mock version check
      const versionProcess = new EventEmitter() as any;
      versionProcess.stdout = new EventEmitter();
      versionProcess.stderr = new EventEmitter();
      mockSpawn.mockReturnValueOnce(versionProcess);
      setImmediate(() => {
        versionProcess.stdout.emit("data", "0.1.0\n");
        versionProcess.emit("close", 0);
      });

      // Mock actual command
      mockSpawn.mockReturnValueOnce(mockProcess);
      setImmediate(() => {
        mockProcess.stdout.emit("data", "{}");
        mockProcess.emit("close", 0);
      });

      await client.exec(["stats"]);

      const lastCall = mockSpawn.mock.calls[mockSpawn.mock.calls.length - 1];
      expect(lastCall[1]).toContain("--db");
      expect(lastCall[1]).toContain("/env/custom.db");
    });

    it("should dynamically resolve dbPath from getSudocodeDir when no explicit dbPath set", async () => {
      // When no static dbPath is configured, exec() should dynamically resolve
      // the dbPath from getSudocodeDir() to ensure the correct project database is used
      const client = new SudocodeClient({ workingDir: "/project" });

      // Mock version check
      const versionProcess = new EventEmitter() as any;
      versionProcess.stdout = new EventEmitter();
      versionProcess.stderr = new EventEmitter();
      mockSpawn.mockReturnValueOnce(versionProcess);
      setImmediate(() => {
        versionProcess.stdout.emit("data", "0.1.0\n");
        versionProcess.emit("close", 0);
      });

      // Mock actual command
      mockSpawn.mockReturnValueOnce(mockProcess);
      setImmediate(() => {
        mockProcess.stdout.emit("data", "{}");
        mockProcess.emit("close", 0);
      });

      await client.exec(["stats"]);

      const lastCall = mockSpawn.mock.calls[mockSpawn.mock.calls.length - 1];
      // Should now ALWAYS add --db flag with dynamically resolved path
      // Default fallback is <workingDir>/.sudocode/cache.db
      expect(lastCall[1]).toContain("--db");
      expect(lastCall[1]).toContain("/project/.sudocode/cache.db");
    });
  });
});
