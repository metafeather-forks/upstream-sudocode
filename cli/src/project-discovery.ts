/**
 * Project Discovery Module
 *
 * Discovers the correct project and sudocodeDir using a two-phase strategy:
 *
 * 1. **Primary**: Walk parent directories looking for `.sudocode` file/dir
 *    (like Git walks for `.git`). Resolve the sudocode data directory,
 *    read `config.json` to get `projectId`.
 *
 * 2. **Fallback**: If `.sudocode` not found, scan the projects.json registry
 *    for matching `projectdir` back-links.
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
import { resolveSudocodeDir } from "./sudocode-file.js";

/**
 * Project information from registry
 */
export interface ProjectInfo {
  id: string;
  name: string;
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
  source: "sudocode-file" | "registry-exact" | "registry-sudocode-dir" | "registry-ancestor" | "generated";
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
 * Resolve the project path for a registered project by reading
 * the projectdir back-link from config.local.json in the project's sudocodeDir.
 * Returns null if the back-link is not set or the file doesn't exist.
 */
export function resolveProjectPath(sudocodeDir: string): string | null {
  try {
    const localConfigPath = path.join(sudocodeDir, "config.local.json");
    if (!fs.existsSync(localConfigPath)) {
      return null;
    }
    const data = JSON.parse(fs.readFileSync(localConfigPath, "utf-8"));
    return data.projectdir || null;
  } catch {
    return null;
  }
}

/**
 * Walk parent directories from `fromPath` looking for a `.sudocode` file or directory,
 * similar to how Git searches for `.git`.
 *
 * Returns the repo root path (directory containing `.sudocode`) and the resolved
 * sudocode data directory, or null if not found.
 */
export function findSudocodeRoot(fromPath: string): { repoRoot: string; sudocodeDir: string } | null {
  let current = path.resolve(fromPath);

  // Walk up until we hit the filesystem root
  const root = path.parse(current).root;
  while (current !== root) {
    try {
      const resolved = resolveSudocodeDir(current);
      if (resolved !== null) {
        return { repoRoot: current, sudocodeDir: resolved };
      }
    } catch {
      // Malformed .sudocode file — skip this directory
    }
    const parent = path.dirname(current);
    if (parent === current) break;
    current = parent;
  }

  return null;
}

/**
 * Read the projectId from a sudocode data directory's config.json.
 * Returns null if the file doesn't exist or projectId is not set.
 */
export function readProjectIdFromConfig(sudocodeDir: string): string | null {
  try {
    const configPath = path.join(sudocodeDir, "config.json");
    if (!fs.existsSync(configPath)) {
      return null;
    }
    const data = JSON.parse(fs.readFileSync(configPath, "utf-8"));
    return data.projectId || null;
  } catch {
    return null;
  }
}

/**
 * Find a registered project containing the given path.
 * Returns null if not found in registry.
 *
 * Lookup priority:
 * 1. Walk parent dirs for .sudocode file/dir (primary — like git)
 * 2. Exact match on project's sudocodeDir in registry
 * 3. Exact match on projectdir back-link (from config.local.json)
 * 4. Ancestor match (longest prefix of projectdir wins)
 */
export function findContainingProject(
  fromPath: string,
  configPath?: string
): ProjectInfo | null {
  // Phase 1: Walk parent dirs for .sudocode (primary mechanism)
  const sudocodeRoot = findSudocodeRoot(fromPath);
  if (sudocodeRoot) {
    // Read projectId from config.json in the sudocode data dir
    const projectId = readProjectIdFromConfig(sudocodeRoot.sudocodeDir);
    if (projectId) {
      // Try to find matching project in registry
      const registry = loadRegistry(configPath);
      if (registry && registry[projectId]) {
        return registry[projectId];
      }
    }
  }

  // Phase 2: Fallback to registry-based lookup
  const registry = loadRegistry(configPath);
  if (!registry) {
    return null;
  }

  const normalizedPath = normalizePath(fromPath);

  // 2a. Check for exact match on project.sudocodeDir
  for (const project of Object.values(registry)) {
    if (project.sudocodeDir && normalizePath(project.sudocodeDir) === normalizedPath) {
      return project;
    }
  }

  // 2b. Check for exact match on projectdir back-link
  for (const project of Object.values(registry)) {
    const projectPath = resolveProjectPath(project.sudocodeDir);
    if (projectPath && normalizePath(projectPath) === normalizedPath) {
      return project;
    }
  }

  // 2c. Find longest prefix match (ancestor) using projectdir back-links
  const projectsWithPaths: Array<{ project: ProjectInfo; projectPath: string }> = [];
  for (const project of Object.values(registry)) {
    const projectPath = resolveProjectPath(project.sudocodeDir);
    if (projectPath) {
      projectsWithPaths.push({ project, projectPath });
    }
  }
  // Sort by path length descending to find most specific match first
  projectsWithPaths.sort((a, b) => b.projectPath.length - a.projectPath.length);

  for (const { project, projectPath } of projectsWithPaths) {
    const normalizedProjectPath = normalizePath(projectPath);
    if (
      normalizedPath === normalizedProjectPath ||
      normalizedPath.startsWith(normalizedProjectPath + path.sep)
    ) {
      return project;
    }
  }

  return null;
}

/**
 * Result of resolving a project by explicit ID from the registry.
 */
export interface ResolvedProject {
  projectId: string;
  sudocodeDir: string;
  dbPath: string;
  projectInfo: ProjectInfo;
}

/**
 * Resolve a project by explicit ID from the registry.
 * Returns null if the project ID is not found in the registry.
 *
 * This is the primary project resolution mechanism for project-scoped operations.
 * No fallback to cwd, env vars, or path discovery.
 */
export function resolveProjectById(
  projectId: string,
  configPath?: string
): ResolvedProject | null {
  const registry = loadRegistry(configPath);
  if (!registry) {
    return null;
  }

  const project = registry[projectId];
  if (!project) {
    return null;
  }

  return {
    projectId: project.id,
    sudocodeDir: project.sudocodeDir,
    dbPath: path.join(project.sudocodeDir, "cache.db"),
    projectInfo: project,
  };
}

/**
 * Discover project from any path.
 * Single call returns projectId, sudocodeDir, and projectPath.
 *
 * Discovery flow:
 * 1. Walk parent dirs for `.sudocode` file/dir → read config.json → get projectId
 * 2. Fallback: scan registry for matching projectdir back-links
 * 3. Last resort: generate ID from path
 *
 * @param fromPath - Path to discover project from (will be normalized)
 * @param configPath - Optional custom registry path
 * @param sudocodeDirOverride - Optional override (e.g., from SUDOCODE_DIR env var) — deprecated
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
          const resolvedPath = resolveProjectPath(project.sudocodeDir) || path.dirname(normalizedOverride);
          return {
            projectId: project.id,
            sudocodeDir: normalizedOverride,
            projectPath: resolvedPath,
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

  // Phase 1: Walk parent dirs for .sudocode file/dir (primary mechanism)
  const sudocodeRoot = findSudocodeRoot(fromPath);
  if (sudocodeRoot) {
    const projectId = readProjectIdFromConfig(sudocodeRoot.sudocodeDir);
    if (projectId) {
      // Try to enrich with registry info
      const registry = loadRegistry(configPath);
      const projectInfo = registry?.[projectId] || undefined;

      return {
        projectId,
        sudocodeDir: sudocodeRoot.sudocodeDir,
        projectPath: sudocodeRoot.repoRoot,
        source: "sudocode-file",
        projectInfo,
      };
    }

    // .sudocode exists but no projectId in config — use generated ID
    return {
      projectId: generateProjectId(sudocodeRoot.repoRoot),
      sudocodeDir: sudocodeRoot.sudocodeDir,
      projectPath: sudocodeRoot.repoRoot,
      source: "sudocode-file",
      warning: "Found .sudocode but config.json has no projectId",
    };
  }

  // Phase 2: Fallback to registry-based discovery
  const project = findContainingProject(fromPath, configPath);

  if (project) {
    // Determine source type based on how we matched
    const resolvedPath = resolveProjectPath(project.sudocodeDir) || path.dirname(project.sudocodeDir);
    const normalizedProjectPath = normalizePath(resolvedPath);
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
      projectPath: resolvedPath,
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
