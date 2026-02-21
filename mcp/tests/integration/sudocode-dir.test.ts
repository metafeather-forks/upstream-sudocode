/**
 * Integration tests for MCP Client getSudocodeDir with real server
 * 
 * Tests dynamic SUDOCODE_DIR resolution when MCP communicates with
 * the sudocode server to discover the current project's sudocodeDir.
 *
 * @group integration
 */

import {
  describe,
  it,
  expect,
  beforeAll,
  afterAll,
  beforeEach,
  afterEach,
} from "vitest";
import { execSync } from "child_process";
import * as path from "path";
import * as fs from "fs";
import * as os from "os";

// Import test helpers from server package
import {
  createTestServer,
  type TestServer,
} from "../../../server/tests/integration/workflow/helpers/workflow-test-server.js";

// Import MCP Client
import { SudocodeClient } from "../../src/client.js";

// Skip integration tests by default (require server setup)
const SKIP_INTEGRATION =
  process.env.SKIP_INTEGRATION_TESTS === "true" || 
  process.env.RUN_INTEGRATION_TESTS !== "true";

describe.skipIf(SKIP_INTEGRATION)("SudocodeClient getSudocodeDir Integration", () => {
  let testDir: string;
  let testServer: TestServer;
  let originalSudocodeDir: string | undefined;

  beforeAll(async () => {
    // Save original env
    originalSudocodeDir = process.env.SUDOCODE_DIR;
    delete process.env.SUDOCODE_DIR;

    // Create temp directory for git repo
    testDir = fs.mkdtempSync(path.join(os.tmpdir(), "sudocode-mcp-integration-"));

    // Initialize as a git repo
    execSync("git init", { cwd: testDir, stdio: "pipe" });
    execSync('git config user.name "Test"', { cwd: testDir, stdio: "pipe" });
    execSync('git config user.email "test@test.com"', {
      cwd: testDir,
      stdio: "pipe",
    });

    // Create .sudocode directory structure
    fs.mkdirSync(path.join(testDir, ".sudocode"), { recursive: true });
    fs.writeFileSync(path.join(testDir, ".sudocode", "issues.jsonl"), "");
    fs.writeFileSync(path.join(testDir, ".sudocode", "specs.jsonl"), "");

    fs.writeFileSync(path.join(testDir, ".gitkeep"), "");
    execSync("git add . && git commit -m 'init'", {
      cwd: testDir,
      stdio: "pipe",
    });
  });

  afterAll(() => {
    // Restore original env
    if (originalSudocodeDir !== undefined) {
      process.env.SUDOCODE_DIR = originalSudocodeDir;
    } else {
      delete process.env.SUDOCODE_DIR;
    }

    if (testDir) {
      fs.rmSync(testDir, { recursive: true, force: true });
    }
  });

  beforeEach(async () => {
    // Clear any env override for each test
    delete process.env.SUDOCODE_DIR;

    testServer = await createTestServer({
      repoPath: testDir,
      mockExecutor: true,
      mockExecutorOptions: {
        defaultDelayMs: 0,
      },
    });
  });

  afterEach(async () => {
    if (testServer) {
      await testServer.shutdown();
    }
  });

  describe("getSudocodeDir with server", () => {
    it("should get sudocodeDir from server's current project", async () => {
      const client = new SudocodeClient({
        serverUrl: testServer.baseUrl,
        workingDir: testDir,
      });

      const sudocodeDir = await client.getSudocodeDir();
      
      // Server should return the project's sudocodeDir
      // Default is <projectPath>/.sudocode
      expect(sudocodeDir).toBe(path.join(testDir, ".sudocode"));
    });

    it("should use override when config.sudocodeDir is set", async () => {
      const customDir = "/custom/override/.sudocode";
      const client = new SudocodeClient({
        serverUrl: testServer.baseUrl,
        workingDir: testDir,
        sudocodeDir: customDir,
      });

      const sudocodeDir = await client.getSudocodeDir();
      
      // Override takes precedence - should NOT call server
      expect(sudocodeDir).toBe(customDir);
    });

    it("should prioritize server response over SUDOCODE_DIR env var", async () => {
      process.env.SUDOCODE_DIR = "/env/override/.sudocode";
      
      const client = new SudocodeClient({
        serverUrl: testServer.baseUrl,
        workingDir: testDir,
      });

      const sudocodeDir = await client.getSudocodeDir();
      
      // Server takes precedence - should return server's sudocodeDir
      // NOT the env var (which is only a fallback when server unavailable)
      expect(sudocodeDir).toBe(path.join(testDir, ".sudocode"));
    });

    it("should fall back to static path when server is unavailable", async () => {
      // Shutdown server to simulate unavailability
      await testServer.shutdown();

      const client = new SudocodeClient({
        serverUrl: testServer.baseUrl, // Points to dead server
        workingDir: testDir,
      });

      const sudocodeDir = await client.getSudocodeDir();
      
      // Should fall back to <workingDir>/.sudocode
      expect(sudocodeDir).toBe(path.join(testDir, ".sudocode"));
    });
  });

  describe("getActiveWorkDir with server", () => {
    it("should return project path from server", async () => {
      const client = new SudocodeClient({
        serverUrl: testServer.baseUrl,
        // Don't set workingDir explicitly to allow server discovery
      });

      const activeDir = await client.getActiveWorkDir();
      
      // Server should return the test project's path
      expect(activeDir).toBe(testDir);
    });

    it("should use explicit workingDir when provided", async () => {
      const explicitDir = "/explicit/working/dir";
      const client = new SudocodeClient({
        serverUrl: testServer.baseUrl,
        workingDir: explicitDir, // Explicitly provided
      });

      const activeDir = await client.getActiveWorkDir();
      
      // Explicit workDir takes precedence - should NOT call server
      expect(activeDir).toBe(explicitDir);
    });
  });
});
