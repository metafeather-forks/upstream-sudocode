import * as fs from 'fs'
import * as path from 'path'
import * as os from 'os'
import * as crypto from 'crypto'
import type { ProjectInfo, ProjectsConfig, ProjectError, Result } from '../types/project.js'
import { Ok, Err } from '../types/project.js'

function getDefaultConfig(): ProjectsConfig {
  return {
    version: 1,
    projects: {},
    recentProjects: [],
    settings: {
      maxRecentProjects: 10,
      autoOpenLastProject: false,
    },
  }
}

/**
 * ProjectRegistry manages the persistent storage of registered projects.
 *
 * Configuration is stored at ~/.config/sudocode/projects.json and includes:
 * - Registered projects with metadata
 * - Recent projects list
 * - User settings
 */
export class ProjectRegistry {
  private configPath: string
  private config: ProjectsConfig

  /**
   * Create a new ProjectRegistry instance
   * @param configPath - Optional custom config file path (defaults to ~/.config/sudocode/projects.json)
   */
  constructor(configPath?: string) {
    this.configPath = configPath || this.getDefaultConfigPath()
    this.config = getDefaultConfig()
  }

  /**
   * Get the default config file path following XDG Base Directory specification
   */
  private getDefaultConfigPath(): string {
    const configDir =
      process.env.XDG_CONFIG_HOME || path.join(os.homedir(), '.config', 'sudocode')
    return path.join(configDir, 'projects.json')
  }

  /**
   * Load configuration from disk. Creates default config if file doesn't exist.
   * @throws {Error} If config file is corrupted or unreadable
   */
  async load(): Promise<Result<void, ProjectError>> {
    try {
      // Ensure config directory exists
      const configDir = path.dirname(this.configPath)
      if (!fs.existsSync(configDir)) {
        fs.mkdirSync(configDir, { recursive: true })
        console.log(`[registry] load: created config directory ${configDir}`)
      }

      // Load existing config or create default
      if (fs.existsSync(this.configPath)) {
        const data = fs.readFileSync(this.configPath, 'utf-8')
        try {
          this.config = JSON.parse(data)

          // Validate config structure
          if (!this.config.version || !this.config.projects || !this.config.settings) {
            throw new Error('Invalid config structure')
          }
          
          const projectCount = Object.keys(this.config.projects).length
          console.log(`[registry] load: loaded ${projectCount} projects from ${this.configPath}`)
        } catch (parseError) {
          // Config is corrupted, backup and create fresh
          const backupPath = `${this.configPath}.backup.${Date.now()}`
          fs.copyFileSync(this.configPath, backupPath)
          console.warn(`[registry] load: corrupted config backed up to: ${backupPath}`)

          this.config = getDefaultConfig()
          await this.save()
        }
      } else {
        // Create default config
        console.log(`[registry] load: no config file found, creating default at ${this.configPath}`)
        await this.save()
      }

      return Ok(undefined)
    } catch (error: any) {
      console.error(`[registry] load: failed to load config from ${this.configPath}:`, error.message)
      if (error.code === 'EACCES') {
        return Err({
          type: 'PERMISSION_DENIED',
          path: this.configPath,
        })
      }
      return Err({
        type: 'UNKNOWN',
        message: error.message,
      })
    }
  }

  /**
   * Save configuration to disk atomically (write to temp file, then rename)
   */
  async save(): Promise<Result<void, ProjectError>> {
    try {
      const tempPath = `${this.configPath}.tmp`
      const data = JSON.stringify(this.config, null, 2)

      // Write to temp file
      fs.writeFileSync(tempPath, data, 'utf-8')

      // Atomic rename
      fs.renameSync(tempPath, this.configPath)

      const projectCount = Object.keys(this.config.projects).length
      console.log(`[registry] save: saved ${projectCount} projects to ${this.configPath}`)

      return Ok(undefined)
    } catch (error: any) {
      console.error(`[registry] save: failed to save config to ${this.configPath}:`, error.message)
      if (error.code === 'EACCES') {
        return Err({
          type: 'PERMISSION_DENIED',
          path: this.configPath,
        })
      }
      return Err({
        type: 'UNKNOWN',
        message: error.message,
      })
    }
  }

  /**
   * Generate a deterministic, human-readable project ID from path
   * Format: <repo-name>-<8-char-hash>
   * Example: sudocode-a1b2c3d4
   */
  generateProjectId(projectPath: string): string {
    // Extract repo name from path
    const repoName = path.basename(projectPath)

    // Create URL-safe version of repo name
    const safeName = repoName
      .toLowerCase()
      .replace(/[^a-z0-9-]/g, '-')
      .replace(/-+/g, '-')
      .replace(/^-|-$/g, '') // Remove leading/trailing dashes
      .slice(0, 32)

    // Generate short hash for uniqueness
    const hash = crypto
      .createHash('sha256')
      .update(projectPath)
      .digest('hex')
      .slice(0, 8)

    return `${safeName}-${hash}`
  }

  /**
   * Register a new project or update existing one
   * @param projectPath - Absolute path to project root directory
   * @param customSudocodeDir - Optional custom sudocode directory path (only used for NEW projects)
   * @returns ProjectInfo for the registered project
   */
  registerProject(projectPath: string, customSudocodeDir?: string): ProjectInfo {
    const projectId = this.generateProjectId(projectPath)
    const now = new Date().toISOString()

    // Check if project already exists
    const existing = this.config.projects[projectId]
    if (existing) {
      // Update existing project - only update lastOpenedAt, NEVER overwrite sudocodeDir
      // sudocodeDir is set once at init time and should not be changed
      existing.lastOpenedAt = now
      this.addToRecent(projectId)
      console.log(`[registry] registerProject (existing): projectId=${projectId}, workingDir=${projectPath}, sudocodeDir=${existing.sudocodeDir}`)
      return existing
    }

    // Create new project info
    // Priority for new projects: customSudocodeDir param > SUDOCODE_DIR env var > default
    const sudocodeDir = customSudocodeDir || 
                        process.env.SUDOCODE_DIR || 
                        path.join(projectPath, '.sudocode')
    
    const projectInfo: ProjectInfo = {
      id: projectId,
      name: path.basename(projectPath),
      path: projectPath,
      sudocodeDir,
      registeredAt: now,
      lastOpenedAt: now,
      favorite: false,
    }

    this.config.projects[projectId] = projectInfo
    this.addToRecent(projectId)

    console.log(`[registry] registerProject (new): projectId=${projectId}, workingDir=${projectPath}, sudocodeDir=${sudocodeDir}`)
    if (customSudocodeDir) {
      console.log(`[registry]   -> sudocodeDir from: customSudocodeDir parameter`)
    } else if (process.env.SUDOCODE_DIR) {
      console.log(`[registry]   -> sudocodeDir from: SUDOCODE_DIR env var`)
    } else {
      console.log(`[registry]   -> sudocodeDir from: default (<projectPath>/.sudocode)`)
    }

    return projectInfo
  }

  /**
   * Unregister a project (remove from registry)
   * @param projectId - Project ID to remove
   * @returns true if project was removed, false if not found
   */
  unregisterProject(projectId: string): boolean {
    if (!this.config.projects[projectId]) {
      console.log(`[registry] unregisterProject: projectId=${projectId} not found`)
      return false
    }

    delete this.config.projects[projectId]

    // Remove from recent projects
    this.config.recentProjects = this.config.recentProjects.filter(
      (id) => id !== projectId
    )

    console.log(`[registry] unregisterProject: projectId=${projectId} removed`)
    return true
  }

  /**
   * Get project info by ID
   */
  getProject(projectId: string): ProjectInfo | null {
    const project = this.config.projects[projectId] || null
    if (project) {
      console.log(`[registry] getProject: projectId=${projectId}, workingDir=${project.path}, sudocodeDir=${project.sudocodeDir}`)
    } else {
      console.log(`[registry] getProject: projectId=${projectId} not found`)
    }
    return project
  }

  /**
   * Get the sudocodeDir for a project path.
   * 
   * Priority (highest to lowest):
   * 1. Stored value in projects.json (if project is registered)
   * 2. SUDOCODE_DIR environment variable
   * 3. Default: <projectPath>/.sudocode
   * 
   * This is the single source of truth for sudocodeDir resolution.
   * All code paths that need sudocodeDir should use this method.
   */
  getSudocodeDir(projectPath: string): string {
    const projectId = this.generateProjectId(projectPath)
    const existing = this.config.projects[projectId]
    
    if (existing) {
      // Use stored value - this is authoritative
      console.log(`[registry] getSudocodeDir: projectId=${projectId}, workingDir=${projectPath}, sudocodeDir=${existing.sudocodeDir} (from stored config)`)
      return existing.sudocodeDir
    }
    
    // Not registered yet - compute from env var or default
    const sudocodeDir = process.env.SUDOCODE_DIR || path.join(projectPath, '.sudocode')
    const source = process.env.SUDOCODE_DIR ? 'SUDOCODE_DIR env var' : 'default'
    console.log(`[registry] getSudocodeDir: projectId=${projectId}, workingDir=${projectPath}, sudocodeDir=${sudocodeDir} (from ${source}, project not registered)`)
    return sudocodeDir
  }

  /**
   * Get all registered projects
   */
  getAllProjects(): ProjectInfo[] {
    const projects = Object.values(this.config.projects)
    console.log(`[registry] getAllProjects: ${projects.length} projects`)
    return projects
  }

  /**
   * Update the lastOpenedAt timestamp for a project
   */
  updateLastOpened(projectId: string): void {
    const project = this.config.projects[projectId]
    if (project) {
      project.lastOpenedAt = new Date().toISOString()
    }
  }

  /**
   * Add a project to the recent projects list
   * Maintains the list at maxRecentProjects size with most recent first
   */
  addToRecent(projectId: string): void {
    // Remove if already in list
    this.config.recentProjects = this.config.recentProjects.filter(
      (id) => id !== projectId
    )

    // Add to front
    this.config.recentProjects.unshift(projectId)

    // Trim to max size
    const maxRecent = this.config.settings.maxRecentProjects
    if (this.config.recentProjects.length > maxRecent) {
      this.config.recentProjects = this.config.recentProjects.slice(0, maxRecent)
    }
  }

  /**
   * Get recent projects (ordered by most recent first)
   */
  getRecentProjects(): ProjectInfo[] {
    return this.config.recentProjects
      .map((id) => this.config.projects[id])
      .filter((p): p is ProjectInfo => p !== undefined)
  }

  /**
   * Update project metadata
   * @param projectId - Project ID to update
   * @param updates - Partial project info to update
   * @returns true if project was updated, false if not found
   */
  updateProject(projectId: string, updates: Partial<Pick<ProjectInfo, 'name' | 'favorite'>>): boolean {
    const project = this.config.projects[projectId]
    if (!project) {
      return false
    }

    // Only allow updating name and favorite fields
    if (updates.name !== undefined) {
      project.name = updates.name
    }
    if (updates.favorite !== undefined) {
      project.favorite = updates.favorite
    }

    return true
  }

  /**
   * Toggle favorite status for a project
   */
  toggleFavorite(projectId: string): boolean {
    const project = this.config.projects[projectId]
    if (!project) {
      return false
    }

    project.favorite = !project.favorite
    return true
  }

  /**
   * Get all favorite projects
   */
  getFavoriteProjects(): ProjectInfo[] {
    return Object.values(this.config.projects).filter((p) => p.favorite)
  }

  /**
   * Update user settings
   */
  updateSettings(settings: Partial<ProjectsConfig['settings']>): void {
    this.config.settings = {
      ...this.config.settings,
      ...settings,
    }
  }

  /**
   * Get current settings
   */
  getSettings(): ProjectsConfig['settings'] {
    return { ...this.config.settings }
  }

  /**
   * Get the config file path
   */
  getConfigPath(): string {
    return this.configPath
  }

  /**
   * Set the UI's current project ID
   * This is the project the UI believes is active, independent of MCP's workDir
   */
  setCurrentProjectId(projectId: string | null): void {
    if (projectId) {
      this.config.currentProjectId = projectId
    } else {
      delete this.config.currentProjectId
    }
  }

  /**
   * Get the UI's current project ID
   */
  getCurrentProjectId(): string | null {
    return this.config.currentProjectId || null
  }
}
