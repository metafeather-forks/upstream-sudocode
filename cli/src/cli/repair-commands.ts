/**
 * Repair command — validates and rebuilds project links.
 *
 * Usage:
 *   sudocode repair          # dry-run: report broken links
 *   sudocode repair --fix    # repair broken links
 *   sudocode repair --rebuild # rebuild projects.json from scratch
 */

import chalk from "chalk";
import * as fs from "fs";
import * as path from "path";
import {
  loadRegistry,
  getRegistryPath,
  resolveProjectPath,
  type ProjectInfo,
  type ProjectsConfig,
} from "../project-discovery.js";
import { resolveSudocodeDir, writeSudocodeFile } from "../sudocode-file.js";

interface RepairIssue {
  projectId: string;
  type: string;
  description: string;
}

interface RepairAction {
  projectId: string;
  type: string;
  description: string;
}

interface RepairResult {
  issues: RepairIssue[];
  actions: RepairAction[];
  rebuilt: boolean;
}

/**
 * Load the full projects config (not just the projects map).
 */
function loadFullConfig(): ProjectsConfig | null {
  const registryPath = getRegistryPath();
  try {
    if (!fs.existsSync(registryPath)) return null;
    const data = fs.readFileSync(registryPath, "utf-8");
    return JSON.parse(data) as ProjectsConfig;
  } catch {
    return null;
  }
}

/**
 * Save the full projects config.
 */
function saveFullConfig(config: ProjectsConfig): void {
  const registryPath = getRegistryPath();
  fs.mkdirSync(path.dirname(registryPath), { recursive: true });
  const tmp = registryPath + ".tmp";
  fs.writeFileSync(tmp, JSON.stringify(config, null, 2), "utf-8");
  fs.renameSync(tmp, registryPath);
}

/**
 * Read projectId from a sudocode dir's config.json.
 */
function readProjectIdFromConfig(sudocodeDir: string): string | null {
  try {
    const configPath = path.join(sudocodeDir, "config.json");
    if (!fs.existsSync(configPath)) return null;
    const data = JSON.parse(fs.readFileSync(configPath, "utf-8"));
    return data.projectId || null;
  } catch {
    return null;
  }
}

/**
 * Perform repair scan and optionally fix issues.
 */
function performRepair(fix: boolean): RepairResult {
  const result: RepairResult = { issues: [], actions: [], rebuilt: false };
  const registry = loadRegistry();
  if (!registry) {
    console.error(chalk.yellow("No projects.json found or it's empty."));
    return result;
  }

  const toRemove: string[] = [];

  for (const [id, project] of Object.entries(registry)) {
    // 1. Check if sudocodeDir exists
    if (!fs.existsSync(project.sudocodeDir)) {
      result.issues.push({
        projectId: id,
        type: "sudocode_dir_missing",
        description: `sudocodeDir ${project.sudocodeDir} does not exist`,
      });
      if (fix) {
        toRemove.push(id);
        result.actions.push({
          projectId: id,
          type: "removed_from_registry",
          description: `removed ${id} from registry (sudocodeDir missing)`,
        });
      }
      continue;
    }

    // 2. Read projectdir back-link
    const projectDir = resolveProjectPath(project.sudocodeDir);
    if (!projectDir) {
      result.issues.push({
        projectId: id,
        type: "back_link_missing",
        description: "config.local.json has no projectdir field",
      });
      continue;
    }

    // 3. Check if projectdir target exists
    if (!fs.existsSync(projectDir)) {
      result.issues.push({
        projectId: id,
        type: "back_link_target_missing",
        description: `projectdir target ${projectDir} does not exist`,
      });
      continue;
    }

    // 4. Check forward link (.sudocode in project dir points back)
    let resolved: string | null = null;
    try {
      resolved = resolveSudocodeDir(projectDir);
    } catch {
      // malformed .sudocode file
    }

    if (resolved === null) {
      result.issues.push({
        projectId: id,
        type: "back_link_target_no_sudocode",
        description: `projectdir target ${projectDir} has no .sudocode`,
      });
      if (fix) {
        try {
          writeSudocodeFile(projectDir, project.sudocodeDir);
          result.actions.push({
            projectId: id,
            type: "created_sudocode_file",
            description: `created .sudocode file in ${projectDir}`,
          });
        } catch {
          // ignore write errors
        }
      }
      continue;
    }

    // 5. Check forward link matches
    if (resolved !== project.sudocodeDir) {
      result.issues.push({
        projectId: id,
        type: "forward_link_mismatch",
        description: `.sudocode in ${projectDir} resolves to ${resolved}, expected ${project.sudocodeDir}`,
      });
      if (fix) {
        try {
          writeSudocodeFile(projectDir, project.sudocodeDir);
          result.actions.push({
            projectId: id,
            type: "fixed_forward_link",
            description: `updated .sudocode file in ${projectDir} to point to ${project.sudocodeDir}`,
          });
        } catch {
          // ignore write errors
        }
      }
    }
  }

  // Apply removals
  if (fix && toRemove.length > 0) {
    const config = loadFullConfig();
    if (config) {
      for (const id of toRemove) {
        delete config.projects[id];
        config.recentProjects = config.recentProjects.filter((rid) => rid !== id);
        if ((config as any).currentProjectId === id) {
          (config as any).currentProjectId = "";
        }
      }
      saveFullConfig(config);
    }
  }

  return result;
}

/**
 * Rebuild projects.json by validating all entries.
 */
function performRebuild(): RepairResult {
  const result: RepairResult = { issues: [], actions: [], rebuilt: true };
  const config = loadFullConfig();
  if (!config) {
    console.error(chalk.yellow("No projects.json found."));
    return result;
  }

  const validProjects: Record<string, ProjectInfo> = {};

  for (const [id, project] of Object.entries(config.projects)) {
    if (!fs.existsSync(project.sudocodeDir)) {
      result.issues.push({
        projectId: id,
        type: "sudocode_dir_missing",
        description: `removed during rebuild: sudocodeDir ${project.sudocodeDir} does not exist`,
      });
      continue;
    }

    const projectId = readProjectIdFromConfig(project.sudocodeDir);
    if (!projectId) {
      result.issues.push({
        projectId: id,
        type: "no_project_id",
        description: "removed during rebuild: no projectId in config.json",
      });
      continue;
    }

    validProjects[projectId] = {
      ...project,
      id: projectId,
    };
  }

  const recentFiltered = config.recentProjects.filter((rid) => rid in validProjects);
  const currentId = (config as any).currentProjectId && (config as any).currentProjectId in validProjects
    ? (config as any).currentProjectId
    : "";

  const newConfig: ProjectsConfig = {
    version: 2,
    projects: validProjects,
    recentProjects: recentFiltered,
    settings: config.settings,
  };
  if (currentId) {
    (newConfig as any).currentProjectId = currentId;
  }

  saveFullConfig(newConfig);
  return result;
}

/**
 * Handle the `sudocode repair` CLI command.
 */
export async function handleRepair(options: { fix?: boolean; rebuild?: boolean; json?: boolean }): Promise<void> {
  const isJson = options.json || false;

  let result: RepairResult;
  if (options.rebuild) {
    result = performRebuild();
  } else {
    result = performRepair(options.fix || false);
  }

  if (isJson) {
    console.log(JSON.stringify(result, null, 2));
    return;
  }

  // Human-readable output
  if (result.rebuilt) {
    console.log(chalk.green("Registry rebuilt."));
  }

  if (result.issues.length === 0 && result.actions.length === 0) {
    console.log(chalk.green("All project links are valid."));
    return;
  }

  if (result.issues.length > 0) {
    console.log(chalk.yellow(`\nFound ${result.issues.length} issue(s):`));
    for (const issue of result.issues) {
      console.log(`  ${chalk.red(issue.type)} [${issue.projectId}]: ${issue.description}`);
    }
  }

  if (result.actions.length > 0) {
    console.log(chalk.green(`\nApplied ${result.actions.length} fix(es):`));
    for (const action of result.actions) {
      console.log(`  ${chalk.green(action.type)} [${action.projectId}]: ${action.description}`);
    }
  }

  if (!options.fix && !options.rebuild && result.issues.length > 0) {
    console.log(chalk.gray("\nRun with --fix to repair these issues."));
  }
}
