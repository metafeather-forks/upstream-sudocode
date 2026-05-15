/**
 * Tests for project discovery module
 */

import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import * as fs from "fs";
import * as path from "path";
import * as os from "os";
import {
  discoverProject,
  findContainingProject,
  findSudocodeRoot,
  readProjectIdFromConfig,
  loadRegistry,
  generateProjectId,
  getConfigDir,
  getRegistryPath,
  resolveProjectById,
  type ProjectInfo,
  type ProjectsConfig,
} from "../../src/project-discovery.js";

// Mock fs module
vi.mock("fs");

// Mock sudocode-file module
vi.mock("../../src/sudocode-file.js", () => ({
  resolveSudocodeDir: vi.fn(() => null),
}));

import { resolveSudocodeDir } from "../../src/sudocode-file.js";

// Mock os module for homedir
vi.mock("os", async () => {
  const actual = await vi.importActual("os");
  return {
    ...actual,
    homedir: vi.fn(() => "/Users/testuser"),
  };
});

describe("Project Discovery", () => {
  const mockHomedir = "/Users/testuser";
  const mockConfigDir = `${mockHomedir}/.config/sudocode`;
  const mockRegistryPath = `${mockConfigDir}/projects.json`;

  const mockProjects: Record<string, ProjectInfo> = {
    "project-a-12345678": {
      id: "project-a-12345678",
      name: "Project A",
      sudocodeDir: "/Users/testuser/projects/project-a/.sudocode",
      registeredAt: "2024-01-01T00:00:00.000Z",
      lastOpenedAt: "2024-01-02T00:00:00.000Z",
    },
    "project-b-87654321": {
      id: "project-b-87654321",
      name: "Project B",
      sudocodeDir: "/Users/testuser/shared-sudocode/project-b",
      registeredAt: "2024-01-01T00:00:00.000Z",
      lastOpenedAt: "2024-01-02T00:00:00.000Z",
    },
    "monorepo-abcd1234": {
      id: "monorepo-abcd1234",
      name: "Monorepo",
      sudocodeDir: "/Users/testuser/projects/monorepo/.sudocode",
      registeredAt: "2024-01-01T00:00:00.000Z",
      lastOpenedAt: "2024-01-02T00:00:00.000Z",
    },
  };

  const mockConfig: ProjectsConfig = {
    version: 2,
    projects: mockProjects,
    recentProjects: ["project-a-12345678", "project-b-87654321"],
    settings: {
      maxRecentProjects: 10,
      autoOpenLastProject: false,
    },
  };

  beforeEach(() => {
    vi.clearAllMocks();
    // Reset environment variables
    delete process.env.XDG_CONFIG_HOME;
    delete process.env.SUDOCODE_DIR;
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe("getConfigDir", () => {
    it("should return default config dir when XDG_CONFIG_HOME is not set", () => {
      const configDir = getConfigDir();
      expect(configDir).toBe(`${mockHomedir}/.config/sudocode`);
    });

    it("should respect XDG_CONFIG_HOME environment variable", () => {
      process.env.XDG_CONFIG_HOME = "/custom/config";
      const configDir = getConfigDir();
      expect(configDir).toBe("/custom/config");
    });
  });

  describe("getRegistryPath", () => {
    it("should return path to projects.json in config dir", () => {
      const registryPath = getRegistryPath();
      expect(registryPath).toBe(`${mockHomedir}/.config/sudocode/projects.json`);
    });
  });

  describe("loadRegistry", () => {
    it("should return null when registry file does not exist", () => {
      vi.mocked(fs.existsSync).mockReturnValue(false);

      const result = loadRegistry();

      expect(result).toBeNull();
    });

    it("should load and parse valid registry file", () => {
      vi.mocked(fs.existsSync).mockReturnValue(true);
      vi.mocked(fs.readFileSync).mockReturnValue(JSON.stringify(mockConfig));

      const result = loadRegistry();

      expect(result).toEqual(mockProjects);
    });

    it("should return null for invalid JSON", () => {
      vi.mocked(fs.existsSync).mockReturnValue(true);
      vi.mocked(fs.readFileSync).mockReturnValue("invalid json {{{");

      const result = loadRegistry();

      expect(result).toBeNull();
    });

    it("should return null for missing version field", () => {
      vi.mocked(fs.existsSync).mockReturnValue(true);
      vi.mocked(fs.readFileSync).mockReturnValue(
        JSON.stringify({ projects: {} })
      );

      const result = loadRegistry();

      expect(result).toBeNull();
    });

    it("should return null for missing projects field", () => {
      vi.mocked(fs.existsSync).mockReturnValue(true);
      vi.mocked(fs.readFileSync).mockReturnValue(JSON.stringify({ version: 1 }));

      const result = loadRegistry();

      expect(result).toBeNull();
    });

    it("should use custom config path when provided", () => {
      const customPath = "/custom/path/projects.json";
      vi.mocked(fs.existsSync).mockReturnValue(true);
      vi.mocked(fs.readFileSync).mockReturnValue(JSON.stringify(mockConfig));

      loadRegistry(customPath);

      expect(fs.existsSync).toHaveBeenCalledWith(customPath);
      expect(fs.readFileSync).toHaveBeenCalledWith(customPath, "utf-8");
    });
  });

  describe("generateProjectId", () => {
    it("should generate deterministic ID from path", () => {
      const id1 = generateProjectId("/Users/test/my-project");
      const id2 = generateProjectId("/Users/test/my-project");

      expect(id1).toBe(id2);
    });

    it("should include sanitized directory name", () => {
      const id = generateProjectId("/Users/test/my-project");

      expect(id).toMatch(/^my-project-[a-f0-9]{8}$/);
    });

    it("should sanitize special characters in directory name", () => {
      const id = generateProjectId("/Users/test/My Project@123!");

      expect(id).toMatch(/^my-project-123-[a-f0-9]{8}$/);
    });

    it("should handle directory names with multiple dashes", () => {
      const id = generateProjectId("/Users/test/my---project---name");

      expect(id).toMatch(/^my-project-name-[a-f0-9]{8}$/);
    });

    it("should truncate long directory names", () => {
      const longName = "a".repeat(50);
      const id = generateProjectId(`/Users/test/${longName}`);

      // Name should be truncated to 32 chars + dash + 8 char hash
      expect(id.length).toBeLessThanOrEqual(32 + 1 + 8);
    });

    it("should generate different IDs for different paths", () => {
      const id1 = generateProjectId("/Users/test/project-a");
      const id2 = generateProjectId("/Users/test/project-b");

      expect(id1).not.toBe(id2);
    });

    it("should resolve relative paths", () => {
      // Both should resolve to the same absolute path
      const cwd = process.cwd();
      const id1 = generateProjectId(cwd);
      const id2 = generateProjectId(".");

      expect(id1).toBe(id2);
    });
  });

  // Mock config.local.json files that map sudocodeDir -> projectdir
  const mockLocalConfigs: Record<string, string> = {
    "/Users/testuser/projects/project-a/.sudocode/config.local.json": JSON.stringify({ projectdir: "/Users/testuser/projects/project-a" }),
    "/Users/testuser/shared-sudocode/project-b/config.local.json": JSON.stringify({ projectdir: "/Users/testuser/projects/project-b" }),
    "/Users/testuser/projects/monorepo/.sudocode/config.local.json": JSON.stringify({ projectdir: "/Users/testuser/projects/monorepo" }),
  };

  /**
   * Setup fs mocks that handle both registry file and config.local.json reads.
   */
  function setupFsMocks() {
    vi.mocked(fs.existsSync).mockReturnValue(true);
    vi.mocked(fs.readFileSync).mockImplementation((filePath: any, _encoding?: any) => {
      const p = String(filePath);
      if (mockLocalConfigs[p]) {
        return mockLocalConfigs[p];
      }
      // Default: return registry JSON
      return JSON.stringify(mockConfig);
    });
  }

  describe("findContainingProject", () => {
    beforeEach(() => {
      setupFsMocks();
    });

    it("should return null when registry is not available", () => {
      vi.mocked(fs.existsSync).mockReturnValue(false);

      const result = findContainingProject("/some/path");

      expect(result).toBeNull();
    });

    it("should find exact match on projectdir back-link", () => {
      const result = findContainingProject(
        "/Users/testuser/projects/project-a"
      );

      expect(result).toEqual(mockProjects["project-a-12345678"]);
    });

    it("should find exact match on sudocodeDir", () => {
      const result = findContainingProject(
        "/Users/testuser/shared-sudocode/project-b"
      );

      expect(result).toEqual(mockProjects["project-b-87654321"]);
    });

    it("should find ancestor project for nested path", () => {
      const result = findContainingProject(
        "/Users/testuser/projects/project-a/src/components"
      );

      expect(result).toEqual(mockProjects["project-a-12345678"]);
    });

    it("should find most specific ancestor (longest prefix)", () => {
      // Monorepo contains project-a path
      const result = findContainingProject(
        "/Users/testuser/projects/monorepo/packages/app"
      );

      expect(result).toEqual(mockProjects["monorepo-abcd1234"]);
    });

    it("should return null for unregistered path", () => {
      const result = findContainingProject(
        "/Users/testuser/unregistered/project"
      );

      expect(result).toBeNull();
    });

    it("should handle paths with trailing slashes", () => {
      const result = findContainingProject(
        "/Users/testuser/projects/project-a/"
      );

      expect(result).toEqual(mockProjects["project-a-12345678"]);
    });
  });

  describe("discoverProject", () => {
    beforeEach(() => {
      setupFsMocks();
    });

    it("should return registry-exact source for exact path match", () => {
      const result = discoverProject("/Users/testuser/projects/project-a");

      expect(result.source).toBe("registry-exact");
      expect(result.projectId).toBe("project-a-12345678");
      expect(result.sudocodeDir).toBe(
        "/Users/testuser/projects/project-a/.sudocode"
      );
      expect(result.projectPath).toBe("/Users/testuser/projects/project-a");
      expect(result.projectInfo).toEqual(mockProjects["project-a-12345678"]);
    });

    it("should return registry-ancestor source for nested path", () => {
      const result = discoverProject(
        "/Users/testuser/projects/project-a/src/lib"
      );

      expect(result.source).toBe("registry-ancestor");
      expect(result.projectId).toBe("project-a-12345678");
      expect(result.sudocodeDir).toBe(
        "/Users/testuser/projects/project-a/.sudocode"
      );
    });

    it("should return generated source for unregistered path", () => {
      const result = discoverProject("/Users/testuser/new-project");

      expect(result.source).toBe("generated");
      expect(result.projectId).toMatch(/^new-project-[a-f0-9]{8}$/);
      expect(result.sudocodeDir).toBe("/Users/testuser/new-project/.sudocode");
      expect(result.projectInfo).toBeUndefined();
    });

    it("should set warning when registry is unavailable", () => {
      vi.mocked(fs.existsSync).mockReturnValue(false);

      const result = discoverProject("/Users/testuser/new-project");

      expect(result.source).toBe("generated");
      expect(result.warning).toBe(
        "Registry file not found or corrupted, using fallback"
      );
    });

    describe("with SUDOCODE_DIR override", () => {
      it("should use override and find matching project", () => {
        const result = discoverProject(
          "/some/random/path",
          undefined,
          "/Users/testuser/projects/project-a/.sudocode"
        );

        expect(result.source).toBe("registry-sudocode-dir");
        expect(result.projectId).toBe("project-a-12345678");
        expect(result.sudocodeDir).toBe(
          "/Users/testuser/projects/project-a/.sudocode"
        );
      });

      it("should use override even when no matching project found", () => {
        const result = discoverProject(
          "/some/random/path",
          undefined,
          "/custom/sudocode/dir"
        );

        expect(result.source).toBe("generated");
        expect(result.sudocodeDir).toBe("/custom/sudocode/dir");
        expect(result.warning).toBe(
          "SUDOCODE_DIR override provided but no matching project in registry"
        );
      });

      it("should derive projectPath from sudocodeDir when using override", () => {
        const result = discoverProject(
          "/some/path",
          undefined,
          "/Users/testuser/my-project/.sudocode"
        );

        expect(result.projectPath).toBe("/Users/testuser/my-project");
      });
    });

    describe("path normalization", () => {
      it("should handle tilde expansion", () => {
        // Mock homedir is /Users/testuser
        const result = discoverProject("~/projects/project-a");

        expect(result.source).toBe("registry-exact");
        expect(result.projectId).toBe("project-a-12345678");
      });

      it("should handle paths with trailing slashes", () => {
        const result = discoverProject("/Users/testuser/projects/project-a/");

        expect(result.source).toBe("registry-exact");
        expect(result.projectId).toBe("project-a-12345678");
      });
    });
  });

  describe("findSudocodeRoot", () => {
    it("should return null when no .sudocode found in any parent", () => {
      vi.mocked(resolveSudocodeDir).mockReturnValue(null);

      const result = findSudocodeRoot("/Users/testuser/projects/some-dir");

      expect(result).toBeNull();
    });

    it("should return repoRoot and sudocodeDir when .sudocode found", () => {
      vi.mocked(resolveSudocodeDir).mockImplementation((dir: string) => {
        if (dir === "/Users/testuser/projects/my-project") {
          return "/Users/testuser/projects/my-project/.sudocode";
        }
        return null;
      });

      const result = findSudocodeRoot("/Users/testuser/projects/my-project/src/lib");

      expect(result).toEqual({
        repoRoot: "/Users/testuser/projects/my-project",
        sudocodeDir: "/Users/testuser/projects/my-project/.sudocode",
      });
    });

    it("should find .sudocode in ancestor directory", () => {
      vi.mocked(resolveSudocodeDir).mockImplementation((dir: string) => {
        if (dir === "/Users/testuser/projects") {
          return "/Users/testuser/projects/.sudocode";
        }
        return null;
      });

      const result = findSudocodeRoot("/Users/testuser/projects/deep/nested/path");

      expect(result).toEqual({
        repoRoot: "/Users/testuser/projects",
        sudocodeDir: "/Users/testuser/projects/.sudocode",
      });
    });

    it("should return closest .sudocode (not a more distant ancestor)", () => {
      vi.mocked(resolveSudocodeDir).mockImplementation((dir: string) => {
        if (dir === "/Users/testuser/projects/mono/packages/app") {
          return "/Users/testuser/projects/mono/packages/app/.sudocode";
        }
        if (dir === "/Users/testuser/projects/mono") {
          return "/Users/testuser/projects/mono/.sudocode";
        }
        return null;
      });

      const result = findSudocodeRoot("/Users/testuser/projects/mono/packages/app/src");

      expect(result).toEqual({
        repoRoot: "/Users/testuser/projects/mono/packages/app",
        sudocodeDir: "/Users/testuser/projects/mono/packages/app/.sudocode",
      });
    });

    it("should skip directories where resolveSudocodeDir throws (malformed .sudocode)", () => {
      vi.mocked(resolveSudocodeDir).mockImplementation((dir: string) => {
        if (dir === "/Users/testuser/projects/bad") {
          throw new Error("malformed .sudocode file");
        }
        if (dir === "/Users/testuser/projects") {
          return "/Users/testuser/projects/.sudocode";
        }
        return null;
      });

      const result = findSudocodeRoot("/Users/testuser/projects/bad/src");

      expect(result).toEqual({
        repoRoot: "/Users/testuser/projects",
        sudocodeDir: "/Users/testuser/projects/.sudocode",
      });
    });
  });

  describe("readProjectIdFromConfig", () => {
    it("should return null when config.json does not exist", () => {
      vi.mocked(fs.existsSync).mockReturnValue(false);

      const result = readProjectIdFromConfig("/some/.sudocode");

      expect(result).toBeNull();
    });

    it("should return projectId from config.json", () => {
      vi.mocked(fs.existsSync).mockReturnValue(true);
      vi.mocked(fs.readFileSync).mockReturnValue(
        JSON.stringify({ projectId: "my-project-abc12345" })
      );

      const result = readProjectIdFromConfig("/some/.sudocode");

      expect(result).toBe("my-project-abc12345");
      expect(fs.readFileSync).toHaveBeenCalledWith(
        "/some/.sudocode/config.json",
        "utf-8"
      );
    });

    it("should return null when config.json has no projectId", () => {
      vi.mocked(fs.existsSync).mockReturnValue(true);
      vi.mocked(fs.readFileSync).mockReturnValue(JSON.stringify({ other: "data" }));

      const result = readProjectIdFromConfig("/some/.sudocode");

      expect(result).toBeNull();
    });

    it("should return null for invalid JSON in config.json", () => {
      vi.mocked(fs.existsSync).mockReturnValue(true);
      vi.mocked(fs.readFileSync).mockReturnValue("not json");

      const result = readProjectIdFromConfig("/some/.sudocode");

      expect(result).toBeNull();
    });
  });

  describe("discoverProject via .sudocode file", () => {
    beforeEach(() => {
      setupFsMocks();
    });

    it("should return sudocode-file source when .sudocode found with projectId in config.json", () => {
      vi.mocked(resolveSudocodeDir).mockImplementation((dir: string) => {
        if (dir === "/Users/testuser/projects/project-a") {
          return "/Users/testuser/projects/project-a/.sudocode";
        }
        return null;
      });
      // config.json read — need to add to readFileSync mock
      const origImpl = vi.mocked(fs.readFileSync).getMockImplementation()!;
      vi.mocked(fs.readFileSync).mockImplementation((filePath: any, encoding?: any) => {
        const p = String(filePath);
        if (p === "/Users/testuser/projects/project-a/.sudocode/config.json") {
          return JSON.stringify({ projectId: "project-a-12345678" });
        }
        return origImpl(filePath, encoding);
      });

      const result = discoverProject("/Users/testuser/projects/project-a/src");

      expect(result.source).toBe("sudocode-file");
      expect(result.projectId).toBe("project-a-12345678");
      expect(result.sudocodeDir).toBe("/Users/testuser/projects/project-a/.sudocode");
      expect(result.projectPath).toBe("/Users/testuser/projects/project-a");
      // Should enrich with registry info
      expect(result.projectInfo).toEqual(mockProjects["project-a-12345678"]);
    });

    it("should return sudocode-file with generated ID when .sudocode exists but config.json has no projectId", () => {
      vi.mocked(resolveSudocodeDir).mockImplementation((dir: string) => {
        if (dir === "/Users/testuser/projects/new-project") {
          return "/Users/testuser/projects/new-project/.sudocode";
        }
        return null;
      });
      // config.json doesn't exist for this project
      const origExistsSync = vi.mocked(fs.existsSync).getMockImplementation()!;
      vi.mocked(fs.existsSync).mockImplementation((p: any) => {
        if (String(p) === "/Users/testuser/projects/new-project/.sudocode/config.json") {
          return false;
        }
        return origExistsSync(p);
      });

      const result = discoverProject("/Users/testuser/projects/new-project");

      expect(result.source).toBe("sudocode-file");
      expect(result.projectId).toMatch(/^new-project-[a-f0-9]{8}$/);
      expect(result.sudocodeDir).toBe("/Users/testuser/projects/new-project/.sudocode");
      expect(result.warning).toBe("Found .sudocode but config.json has no projectId");
    });

    it("should take priority over registry-based discovery", () => {
      // .sudocode resolves with a DIFFERENT projectId than registry would match
      vi.mocked(resolveSudocodeDir).mockImplementation((dir: string) => {
        if (dir === "/Users/testuser/projects/project-a") {
          return "/Users/testuser/projects/project-a/.sudocode";
        }
        return null;
      });
      const origImpl = vi.mocked(fs.readFileSync).getMockImplementation()!;
      vi.mocked(fs.readFileSync).mockImplementation((filePath: any, encoding?: any) => {
        const p = String(filePath);
        if (p === "/Users/testuser/projects/project-a/.sudocode/config.json") {
          return JSON.stringify({ projectId: "custom-override-id" });
        }
        return origImpl(filePath, encoding);
      });

      const result = discoverProject("/Users/testuser/projects/project-a");

      // Should use sudocode-file, not registry-exact
      expect(result.source).toBe("sudocode-file");
      expect(result.projectId).toBe("custom-override-id");
    });
  });

  describe("resolveProjectById", () => {
    beforeEach(() => {
      vi.mocked(fs.existsSync).mockReturnValue(true);
      vi.mocked(fs.readFileSync).mockReturnValue(JSON.stringify(mockConfig));
    });

    it("should resolve a registered project by ID", () => {
      const result = resolveProjectById("project-a-12345678");

      expect(result).not.toBeNull();
      expect(result!.projectId).toBe("project-a-12345678");
      expect(result!.sudocodeDir).toBe("/Users/testuser/projects/project-a/.sudocode");
      expect(result!.dbPath).toBe("/Users/testuser/projects/project-a/.sudocode/cache.db");
      expect(result!.projectInfo).toEqual(mockProjects["project-a-12345678"]);
    });

    it("should return null for non-existent project ID", () => {
      const result = resolveProjectById("nonexistent-project-00000000");

      expect(result).toBeNull();
    });

    it("should return null when registry is unavailable", () => {
      vi.mocked(fs.existsSync).mockReturnValue(false);

      const result = resolveProjectById("project-a-12345678");

      expect(result).toBeNull();
    });

    it("should return null for empty string project ID", () => {
      const result = resolveProjectById("");

      expect(result).toBeNull();
    });

    it("should resolve project with non-default sudocodeDir", () => {
      const result = resolveProjectById("project-b-87654321");

      expect(result).not.toBeNull();
      expect(result!.sudocodeDir).toBe("/Users/testuser/shared-sudocode/project-b");
      expect(result!.dbPath).toBe("/Users/testuser/shared-sudocode/project-b/cache.db");
    });

    it("should use custom config path when provided", () => {
      const customPath = "/custom/path/projects.json";
      resolveProjectById("project-a-12345678", customPath);

      expect(fs.existsSync).toHaveBeenCalledWith(customPath);
    });
  });
});
