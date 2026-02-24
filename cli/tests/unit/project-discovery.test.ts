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
  loadRegistry,
  generateProjectId,
  getConfigDir,
  getRegistryPath,
  type ProjectInfo,
  type ProjectsConfig,
} from "../../src/project-discovery.js";

// Mock fs module
vi.mock("fs");

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
      path: "/Users/testuser/projects/project-a",
      sudocodeDir: "/Users/testuser/projects/project-a/.sudocode",
      registeredAt: "2024-01-01T00:00:00.000Z",
      lastOpenedAt: "2024-01-02T00:00:00.000Z",
    },
    "project-b-87654321": {
      id: "project-b-87654321",
      name: "Project B",
      path: "/Users/testuser/projects/project-b",
      sudocodeDir: "/Users/testuser/shared-sudocode/project-b",
      registeredAt: "2024-01-01T00:00:00.000Z",
      lastOpenedAt: "2024-01-02T00:00:00.000Z",
    },
    "monorepo-abcd1234": {
      id: "monorepo-abcd1234",
      name: "Monorepo",
      path: "/Users/testuser/projects/monorepo",
      sudocodeDir: "/Users/testuser/projects/monorepo/.sudocode",
      registeredAt: "2024-01-01T00:00:00.000Z",
      lastOpenedAt: "2024-01-02T00:00:00.000Z",
    },
  };

  const mockConfig: ProjectsConfig = {
    version: 1,
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

  describe("findContainingProject", () => {
    beforeEach(() => {
      vi.mocked(fs.existsSync).mockReturnValue(true);
      vi.mocked(fs.readFileSync).mockReturnValue(JSON.stringify(mockConfig));
    });

    it("should return null when registry is not available", () => {
      vi.mocked(fs.existsSync).mockReturnValue(false);

      const result = findContainingProject("/some/path");

      expect(result).toBeNull();
    });

    it("should find exact match on project path", () => {
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
      vi.mocked(fs.existsSync).mockReturnValue(true);
      vi.mocked(fs.readFileSync).mockReturnValue(JSON.stringify(mockConfig));
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
});
