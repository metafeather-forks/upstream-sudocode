/**
 * Project Discovery Module
 *
 * Discovers the correct project and sudocodeDir from any path by reading
 * the ~/.config/sudocode/projects.json registry directly (offline-first).
 *
 * This enables the CLI to work correctly when:
 * - Called from nested directories within a project
 * - Called via --working-dir flag pointing to a different location
 * - No explicit database path is provided
 */

import * as fs from "fs";
import * as path from "path";
import * as os from "os";
import * as crypto from "crypto";

/**
 * Project information from registry
 */
export interface ProjectInfo {
  id: string;
  name: string;
  path: string;
  sudocodeDir: string;
  registeredAt: string;
  lastOpenedAt: string;
  favorite?: boolean;
}

/**
 * Projects configuration schema
 */
export interface ProjectsConfig {
  version: number;
  projects: Record<string, ProjectInfo>;
  recentProjects: string[];
  settings: {
    maxRecentProjects: number;
    autoOpenLastProject: boolean;
  };
}

/**
 * Result of project discovery
 */
export interface DiscoveryResult {
  projectId: string;
  sudocodeDir: string;
  projectPath: string;
  source: "registry-exact" | "registry-sudocode-dir" | "registry-ancestor" | "generated";
  projectInfo?: ProjectInfo;
  warning?: string;
}

/**
 * Normalize a path for consistent comparison.
 * - Expands ~ to home directory
 * - Resolves to absolute path
 * - Removes trailing slashes
 * - Normalizes case on Windows
 */
function normalizePath(p: string): string {
  // Expand ~ to home directory
  let expanded = p;
  if (p === "~") {
    expanded = os.homedir();
  } else if (p.startsWith("~/")) {
    expanded = path.join(os.homedir(), p.slice(2));
  }

  const resolved = path.resolve(expanded);
  const normalized = path.normalize(resolved).replace(/[/\\]+$/, "");
  return process.platform === "win32" ? normalized.toLowerCase() : normalized;
}

/**
 * Get the sudocode config directory.
 * Respects XDG_CONFIG_HOME on Linux/macOS.
 */
export function getConfigDir(): string {
  return process.env.XDG_CONFIG_HOME || path.join(os.homedir(), ".config", "sudocode");
}

/**
 * Get the default registry config path.
 * Returns: ~/.config/sudocode/projects.json (or XDG_CONFIG_HOME variant)
 */
export function getRegistryPath(): string {
  return path.join(getConfigDir(), "projects.json");
}

/**
 * Load projects from registry file.
 * Returns null if file doesn't exist or is invalid.
 */
export function loadRegistry(configPath?: string): Record<string, ProjectInfo> | null {
  const registryPath = configPath || getRegistryPath();

  try {
    if (!fs.existsSync(registryPath)) {
      return null;
    }

    const data = fs.readFileSync(registryPath, "utf-8");
    const config = JSON.parse(data) as ProjectsConfig;

    // Validate config structure
    if (!config.version || !config.projects) {
      return null;
    }

    return config.projects;
  } catch {
    return null;
  }
}

/**
 * Generate a deterministic project ID from a path.
 * Format: <sanitized-dir-name>-<8-char-sha256>
 *
 * Uses the same algorithm as server's ProjectRegistry and MCP.
 */
export function generateProjectId(projectPath: string): string {
  const absolutePath = path.resolve(projectPath);
  const repoName = path.basename(absolutePath);

  // Create URL-safe version of repo name
  const safeName = repoName
    .toLowerCase()
    .replace(/[^a-z0-9-]/g, "-")
    .replace(/-+/g, "-")
    .replace(/^-|-$/g, "")
    .slice(0, 32);

  // Generate short hash for uniqueness
  const hash = crypto.createHash("sha256").update(absolutePath).digest("hex").slice(0, 8);

  return `${safeName}-${hash}`;
}

/**
 * Find a registered project containing the given path.
 * Returns null if not found in registry.
 *
 * Lookup priority:
 * 1. Exact match on project.path
 * 2. Exact match on project.sudocodeDir
 * 3. Ancestor match (longest prefix wins)
 */
export function findContainingProject(
  fromPath: string,
  configPath?: string
): ProjectInfo | null {
  const registry = loadRegistry(configPath);
  if (!registry) {
    return null;
  }

  const normalizedPath = normalizePath(fromPath);

  // 1. Check for exact match on project.path
  for (const project of Object.values(registry)) {
    if (project.path && normalizePath(project.path) === normalizedPath) {
      return project;
    }
  }

  // 2. Check for exact match on project.sudocodeDir
  for (const project of Object.values(registry)) {
    if (project.sudocodeDir && normalizePath(project.sudocodeDir) === normalizedPath) {
      return project;
    }
  }

  // 3. Find longest prefix match (ancestor)
  // Sort by path length descending to find most specific match first
  const sortedProjects = Object.values(registry)
    .filter((p) => p.path)
    .sort((a, b) => b.path.length - a.path.length);

  for (const project of sortedProjects) {
    const projectPath = normalizePath(project.path);
    if (
      normalizedPath === projectPath ||
      normalizedPath.startsWith(projectPath + path.sep)
    ) {
      return project;
    }
  }

  return null;
}

/**
 * Discover project from any path.
 * Single call returns projectId, sudocodeDir, and projectPath.
 *
 * @param fromPath - Path to discover project from (will be normalized)
 * @param configPath - Optional custom registry path
 * @param sudocodeDirOverride - Optional override (e.g., from SUDOCODE_DIR env var)
 */
export function discoverProject(
  fromPath: string,
  configPath?: string,
  sudocodeDirOverride?: string
): DiscoveryResult {
  const normalizedFromPath = normalizePath(fromPath);

  // If SUDOCODE_DIR override is provided, use it but still try to find projectId
  if (sudocodeDirOverride) {
    const normalizedOverride = normalizePath(sudocodeDirOverride);

    // Try to find a project that matches the override directory
    const registry = loadRegistry(configPath);
    if (registry) {
      for (const project of Object.values(registry)) {
        if (project.sudocodeDir && normalizePath(project.sudocodeDir) === normalizedOverride) {
          return {
            projectId: project.id,
            sudocodeDir: normalizedOverride,
            projectPath: project.path,
            source: "registry-sudocode-dir",
            projectInfo: project,
          };
        }
      }
    }

    // Override provided but no matching project found
    // Derive projectPath from sudocodeDir (assume .sudocode is in project root)
    const derivedProjectPath = path.dirname(normalizedOverride);
    return {
      projectId: generateProjectId(derivedProjectPath),
      sudocodeDir: normalizedOverride,
      projectPath: derivedProjectPath,
      source: "generated",
      warning: "SUDOCODE_DIR override provided but no matching project in registry",
    };
  }

  // Try to find containing project in registry
  const project = findContainingProject(fromPath, configPath);

  if (project) {
    // Determine source type based on how we matched
    const normalizedProjectPath = normalizePath(project.path);
    let source: DiscoveryResult["source"];

    if (normalizedFromPath === normalizedProjectPath) {
      source = "registry-exact";
    } else if (project.sudocodeDir && normalizedFromPath === normalizePath(project.sudocodeDir)) {
      source = "registry-sudocode-dir";
    } else {
      source = "registry-ancestor";
    }

    return {
      projectId: project.id,
      sudocodeDir: project.sudocodeDir,
      projectPath: project.path,
      source,
      projectInfo: project,
    };
  }

  // No matching project found - fall back to generated ID
  const absolutePath = path.resolve(fromPath);

  // Check if registry file exists but just doesn't have this project
  const registry = loadRegistry(configPath);
  const warning = registry === null
    ? "Registry file not found or corrupted, using fallback"
    : undefined;

  return {
    projectId: generateProjectId(absolutePath),
    sudocodeDir: path.join(absolutePath, ".sudocode"),
    projectPath: absolutePath,
    source: "generated",
    warning,
  };
}
