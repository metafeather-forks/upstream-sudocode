import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import * as fs from 'fs'
import * as path from 'path'
import * as os from 'os'
import { ProjectRegistry } from '../../../src/services/project-registry.js'
import type { ProjectsConfig } from '../../../src/types/project.js'

describe('ProjectRegistry', () => {
  let tempDir: string
  let configPath: string
  let registry: ProjectRegistry

  beforeEach(() => {
    // Create a unique temporary directory for each test
    tempDir = fs.mkdtempSync(path.join(os.tmpdir(), `sudocode-test-`))
    configPath = path.join(tempDir, 'projects.json')
    registry = new ProjectRegistry(configPath)
  })

  afterEach(() => {
    // Clean up temp directory
    if (fs.existsSync(tempDir)) {
      fs.rmSync(tempDir, { recursive: true, force: true })
    }
  })

  describe('initialization', () => {
    it('should create config directory if it does not exist', async () => {
      const result = await registry.load()
      expect(result.ok).toBe(true)
      expect(fs.existsSync(path.dirname(configPath))).toBe(true)
    })

    it('should create default config file on first load', async () => {
      const result = await registry.load()
      expect(result.ok).toBe(true)
      expect(fs.existsSync(configPath)).toBe(true)

      const config = JSON.parse(fs.readFileSync(configPath, 'utf-8')) as ProjectsConfig
      expect(config.version).toBe(1)
      expect(config.projects).toEqual({})
      expect(config.recentProjects).toEqual([])
      expect(config.settings.maxRecentProjects).toBe(10)
    })

    it('should load existing config file', async () => {
      // Create a config file
      const existingConfig: ProjectsConfig = {
        version: 1,
        projects: {
          'test-12345678': {
            id: 'test-12345678',
            name: 'test',
            path: '/path/to/test',
            sudocodeDir: '/path/to/test/.sudocode',
            registeredAt: '2025-01-01T00:00:00.000Z',
            lastOpenedAt: '2025-01-01T00:00:00.000Z',
            favorite: false,
          },
        },
        recentProjects: ['test-12345678'],
        settings: {
          maxRecentProjects: 10,
          autoOpenLastProject: false,
        },
      }
      fs.writeFileSync(configPath, JSON.stringify(existingConfig))

      const result = await registry.load()
      expect(result.ok).toBe(true)

      const project = registry.getProject('test-12345678')
      expect(project).not.toBeNull()
      expect(project?.name).toBe('test')
    })

    it('should handle corrupted config file gracefully', async () => {
      // Write invalid JSON
      fs.writeFileSync(configPath, 'invalid json{{{')

      const result = await registry.load()
      expect(result.ok).toBe(true)

      // Should create backup
      const backupFiles = fs.readdirSync(tempDir).filter((f) => f.includes('backup'))
      expect(backupFiles.length).toBe(1)

      // Should have fresh config
      const config = JSON.parse(fs.readFileSync(configPath, 'utf-8')) as ProjectsConfig
      expect(config.projects).toEqual({})
    })
  })

  describe('generateProjectId', () => {
    it('should generate deterministic project IDs', () => {
      const path1 = '/Users/alex/repos/sudocode'
      const id1 = registry.generateProjectId(path1)
      const id2 = registry.generateProjectId(path1)

      expect(id1).toBe(id2)
    })

    it('should generate unique IDs for different paths', () => {
      const path1 = '/Users/alex/repos/sudocode'
      const path2 = '/Users/alex/repos/other-repo'

      const id1 = registry.generateProjectId(path1)
      const id2 = registry.generateProjectId(path2)

      expect(id1).not.toBe(id2)
    })

    it('should generate URL-safe IDs', () => {
      const pathWithSpaces = '/Users/alex/My Projects/Some App'
      const id = registry.generateProjectId(pathWithSpaces)

      // Should not contain spaces or special characters except dash
      expect(id).toMatch(/^[a-z0-9-]+$/)
    })

    it('should include repo name in ID', () => {
      const projectPath = '/Users/alex/repos/my-awesome-project'
      const id = registry.generateProjectId(projectPath)

      expect(id).toContain('my-awesome-project')
    })

    it('should append hash to prevent collisions', () => {
      const projectPath = '/Users/alex/repos/sudocode'
      const id = registry.generateProjectId(projectPath)

      // Format should be: <name>-<8-char-hash>
      const parts = id.split('-')
      const hash = parts[parts.length - 1]
      expect(hash).toHaveLength(8)
      expect(hash).toMatch(/^[a-f0-9]{8}$/)
    })
  })

  describe('registerProject', () => {
    it('should register a new project', async () => {
      await registry.load()

      const projectPath = '/Users/alex/repos/test-project'
      const projectInfo = registry.registerProject(projectPath)

      expect(projectInfo.id).toBeTruthy()
      expect(projectInfo.name).toBe('test-project')
      expect(projectInfo.path).toBe(projectPath)
      expect(projectInfo.sudocodeDir).toBe(path.join(projectPath, '.sudocode'))
      expect(projectInfo.registeredAt).toBeTruthy()
      expect(projectInfo.favorite).toBe(false)
    })

    it('should update lastOpenedAt for existing project', async () => {
      await registry.load()

      const projectPath = '/Users/alex/repos/test-project'
      const project1 = registry.registerProject(projectPath)
      const timestamp1 = project1.lastOpenedAt

      // Wait to ensure different timestamp
      await new Promise((resolve) => setTimeout(resolve, 100))

      const project2 = registry.registerProject(projectPath)

      expect(project1.id).toBe(project2.id)
      expect(project2.lastOpenedAt).not.toBe(timestamp1)
      expect(new Date(project2.lastOpenedAt).getTime()).toBeGreaterThan(
        new Date(timestamp1).getTime()
      )
    })

    it('should add project to recent list on registration', async () => {
      await registry.load()

      const projectPath = '/Users/alex/repos/test-project'
      registry.registerProject(projectPath)

      const recent = registry.getRecentProjects()
      expect(recent).toHaveLength(1)
      expect(recent[0].path).toBe(projectPath)
    })
  })

  describe('unregisterProject', () => {
    it('should remove project from registry', async () => {
      await registry.load()

      const projectPath = '/Users/alex/repos/test-project'
      const project = registry.registerProject(projectPath)

      const removed = registry.unregisterProject(project.id)
      expect(removed).toBe(true)

      const retrieved = registry.getProject(project.id)
      expect(retrieved).toBeNull()
    })

    it('should remove project from recent list', async () => {
      await registry.load()

      const projectPath = '/Users/alex/repos/test-project'
      const project = registry.registerProject(projectPath)

      registry.unregisterProject(project.id)

      const recent = registry.getRecentProjects()
      expect(recent).toHaveLength(0)
    })

    it('should return false for non-existent project', async () => {
      await registry.load()

      const removed = registry.unregisterProject('non-existent-id')
      expect(removed).toBe(false)
    })
  })

  describe('getProject and getAllProjects', () => {
    it('should retrieve project by ID', async () => {
      await registry.load()

      const projectPath = '/Users/alex/repos/test-project'
      const registered = registry.registerProject(projectPath)

      const retrieved = registry.getProject(registered.id)
      expect(retrieved).not.toBeNull()
      expect(retrieved?.id).toBe(registered.id)
      expect(retrieved?.path).toBe(projectPath)
    })

    it('should return null for non-existent project', async () => {
      await registry.load()

      const retrieved = registry.getProject('non-existent-id')
      expect(retrieved).toBeNull()
    })

    it('should return all registered projects', async () => {
      // Create a fresh registry for this test to avoid retry issues
      const testTempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'sudocode-getall-'))
      const testConfigPath = path.join(testTempDir, 'projects.json')
      const testRegistry = new ProjectRegistry(testConfigPath)

      try {
        await testRegistry.load()

        testRegistry.registerProject('/Users/alex/repos/getall-project1')
        testRegistry.registerProject('/Users/alex/repos/getall-project2')
        testRegistry.registerProject('/Users/alex/repos/getall-project3')

        const all = testRegistry.getAllProjects()
        expect(all).toHaveLength(3)
      } finally {
        // Clean up
        if (fs.existsSync(testTempDir)) {
          fs.rmSync(testTempDir, { recursive: true, force: true })
        }
      }
    })
  })

  describe('recent projects', () => {
    it('should maintain recent projects list', async () => {
      await registry.load()

      const project1 = registry.registerProject('/Users/alex/repos/project1')
      const project2 = registry.registerProject('/Users/alex/repos/project2')
      const project3 = registry.registerProject('/Users/alex/repos/project3')

      const recent = registry.getRecentProjects()
      expect(recent).toHaveLength(3)

      // Most recent should be first
      expect(recent[0].id).toBe(project3.id)
      expect(recent[1].id).toBe(project2.id)
      expect(recent[2].id).toBe(project1.id)
    })

    it('should move project to front when re-added to recent', async () => {
      await registry.load()

      const project1 = registry.registerProject('/Users/alex/repos/project1')
      const project2 = registry.registerProject('/Users/alex/repos/project2')

      // Add project1 again
      registry.addToRecent(project1.id)

      const recent = registry.getRecentProjects()
      expect(recent[0].id).toBe(project1.id)
      expect(recent[1].id).toBe(project2.id)
    })

    it('should limit recent projects to maxRecentProjects', async () => {
      await registry.load()
      registry.updateSettings({ maxRecentProjects: 3 })

      // Register 5 projects
      for (let i = 1; i <= 5; i++) {
        registry.registerProject(`/Users/alex/repos/project${i}`)
      }

      const recent = registry.getRecentProjects()
      expect(recent).toHaveLength(3)
    })

    it('should filter out deleted projects from recent list', async () => {
      await registry.load()

      const project1 = registry.registerProject('/Users/alex/repos/project1')
      registry.registerProject('/Users/alex/repos/project2')

      // Delete project1
      registry.unregisterProject(project1.id)

      const recent = registry.getRecentProjects()
      expect(recent).toHaveLength(1)
      expect(recent[0].id).not.toBe(project1.id)
    })
  })

  describe('updateProject', () => {
    it('should update project name', async () => {
      await registry.load()

      const project = registry.registerProject('/Users/alex/repos/test-project')
      expect(project.name).toBe('test-project')

      const updated = registry.updateProject(project.id, { name: 'My Awesome Project' })
      expect(updated).toBe(true)

      const retrieved = registry.getProject(project.id)
      expect(retrieved?.name).toBe('My Awesome Project')
    })

    it('should update project name with special characters', async () => {
      await registry.load()

      const project = registry.registerProject('/Users/alex/repos/test-project')

      // Valid special characters that should work
      const validNames = [
        'Project-Name',
        'My_Project',
        'Project (v2)',
        'Project [2024]',
        'Project.Name',
        'Project & Co',
      ]

      for (const name of validNames) {
        const updated = registry.updateProject(project.id, { name })
        expect(updated).toBe(true)
        const retrieved = registry.getProject(project.id)
        expect(retrieved?.name).toBe(name)
      }
    })

    it('should update favorite status', async () => {
      await registry.load()

      const project = registry.registerProject('/Users/alex/repos/test-project')
      expect(project.favorite).toBe(false)

      const updated = registry.updateProject(project.id, { favorite: true })
      expect(updated).toBe(true)

      const retrieved = registry.getProject(project.id)
      expect(retrieved?.favorite).toBe(true)
    })

    it('should update both name and favorite', async () => {
      await registry.load()

      const project = registry.registerProject('/Users/alex/repos/test-project')

      const updated = registry.updateProject(project.id, {
        name: 'Renamed Project',
        favorite: true,
      })
      expect(updated).toBe(true)

      const retrieved = registry.getProject(project.id)
      expect(retrieved?.name).toBe('Renamed Project')
      expect(retrieved?.favorite).toBe(true)
    })

    it('should return false for non-existent project', async () => {
      await registry.load()

      const updated = registry.updateProject('non-existent-id', { name: 'Test' })
      expect(updated).toBe(false)
    })

    it('should handle empty updates object', async () => {
      await registry.load()

      const project = registry.registerProject('/Users/alex/repos/test-project')
      const originalName = project.name

      const updated = registry.updateProject(project.id, {})
      expect(updated).toBe(true)

      const retrieved = registry.getProject(project.id)
      expect(retrieved?.name).toBe(originalName)
    })
  })

  describe('favorites', () => {
    it('should toggle favorite status', async () => {
      await registry.load()

      const project = registry.registerProject('/Users/alex/repos/test-project')
      expect(project.favorite).toBe(false)

      registry.toggleFavorite(project.id)
      const updated = registry.getProject(project.id)
      expect(updated?.favorite).toBe(true)

      registry.toggleFavorite(project.id)
      const toggled = registry.getProject(project.id)
      expect(toggled?.favorite).toBe(false)
    })

    it('should return false when toggling non-existent project', async () => {
      await registry.load()

      const result = registry.toggleFavorite('non-existent-id')
      expect(result).toBe(false)
    })

    it('should get all favorite projects', async () => {
      await registry.load()

      const project1 = registry.registerProject('/Users/alex/repos/project1')
      const project2 = registry.registerProject('/Users/alex/repos/project2')
      registry.registerProject('/Users/alex/repos/project3')

      registry.toggleFavorite(project1.id)
      registry.toggleFavorite(project2.id)

      const favorites = registry.getFavoriteProjects()
      expect(favorites).toHaveLength(2)
      expect(favorites.some((p) => p.id === project1.id)).toBe(true)
      expect(favorites.some((p) => p.id === project2.id)).toBe(true)
    })
  })

  describe('persistence', () => {
    it('should persist changes to disk', async () => {
      // Create a fresh registry for this test to avoid retry issues
      const testTempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'sudocode-persist-'))
      const testConfigPath = path.join(testTempDir, 'projects.json')
      const testRegistry = new ProjectRegistry(testConfigPath)

      try {
        await testRegistry.load()

        const projectPath = '/Users/alex/repos/test-project-persist'
        testRegistry.registerProject(projectPath)

        const saveResult = await testRegistry.save()
        expect(saveResult.ok).toBe(true)

        // Create new registry instance and load
        const registry2 = new ProjectRegistry(testConfigPath)
        await registry2.load()

        const all = registry2.getAllProjects()
        expect(all).toHaveLength(1)
        expect(all[0].path).toBe(projectPath)
      } finally {
        // Clean up
        if (fs.existsSync(testTempDir)) {
          fs.rmSync(testTempDir, { recursive: true, force: true })
        }
      }
    })

    it('should save atomically (write to temp, then rename)', async () => {
      await registry.load()

      registry.registerProject('/Users/alex/repos/test-project')
      const saveResult = await registry.save()

      expect(saveResult.ok).toBe(true)

      // Temp file should not exist after save
      const tempPath = `${configPath}.tmp`
      expect(fs.existsSync(tempPath)).toBe(false)

      // Config file should exist
      expect(fs.existsSync(configPath)).toBe(true)
    })

    it('should preserve all data across save/load cycle', async () => {
      await registry.load()

      const project = registry.registerProject('/Users/alex/repos/test-project')
      registry.toggleFavorite(project.id)
      registry.updateSettings({ maxRecentProjects: 5 })

      await registry.save()

      // Load in new instance
      const registry2 = new ProjectRegistry(configPath)
      await registry2.load()

      const loaded = registry2.getProject(project.id)
      expect(loaded?.favorite).toBe(true)

      const settings = registry2.getSettings()
      expect(settings.maxRecentProjects).toBe(5)
    })
  })

  describe('settings', () => {
    it('should update settings', async () => {
      await registry.load()

      registry.updateSettings({
        maxRecentProjects: 20,
        autoOpenLastProject: true,
      })

      const settings = registry.getSettings()
      expect(settings.maxRecentProjects).toBe(20)
      expect(settings.autoOpenLastProject).toBe(true)
    })

    it('should support partial settings updates', async () => {
      await registry.load()

      registry.updateSettings({ maxRecentProjects: 15 })

      const settings = registry.getSettings()
      expect(settings.maxRecentProjects).toBe(15)
      expect(settings.autoOpenLastProject).toBe(false) // Should keep default
    })
  })

  describe('updateLastOpened', () => {
    it('should update lastOpenedAt timestamp', async () => {
      await registry.load()

      const project = registry.registerProject('/Users/alex/repos/test-project')
      const originalTimestamp = project.lastOpenedAt

      // Wait a bit
      await new Promise((resolve) => setTimeout(resolve, 10))

      registry.updateLastOpened(project.id)
      const updated = registry.getProject(project.id)

      expect(updated?.lastOpenedAt).not.toBe(originalTimestamp)
    })

    it('should do nothing for non-existent project', async () => {
      await registry.load()

      // Should not throw
      registry.updateLastOpened('non-existent-id')
    })
  })

  describe('SUDOCODE_DIR resolution in registerProject', () => {
    let originalSudocodeDir: string | undefined

    beforeEach(() => {
      originalSudocodeDir = process.env.SUDOCODE_DIR
    })

    afterEach(() => {
      if (originalSudocodeDir !== undefined) {
        process.env.SUDOCODE_DIR = originalSudocodeDir
      } else {
        delete process.env.SUDOCODE_DIR
      }
    })

    it('should use customSudocodeDir param when provided', async () => {
      delete process.env.SUDOCODE_DIR
      await registry.load()

      const projectPath = '/Users/alex/repos/test-project'
      const customDir = '/custom/location/.sudocode'
      const projectInfo = registry.registerProject(projectPath, customDir)

      expect(projectInfo.sudocodeDir).toBe(customDir)
    })

    it('should use SUDOCODE_DIR env var when no customSudocodeDir provided', async () => {
      process.env.SUDOCODE_DIR = '/env/custom/.sudocode'
      await registry.load()

      const projectPath = '/Users/alex/repos/test-project'
      const projectInfo = registry.registerProject(projectPath)

      expect(projectInfo.sudocodeDir).toBe('/env/custom/.sudocode')
    })

    it('should default to <projectPath>/.sudocode when neither provided', async () => {
      delete process.env.SUDOCODE_DIR
      await registry.load()

      const projectPath = '/Users/alex/repos/test-project'
      const projectInfo = registry.registerProject(projectPath)

      expect(projectInfo.sudocodeDir).toBe(path.join(projectPath, '.sudocode'))
    })

    it('should prioritize customSudocodeDir over SUDOCODE_DIR env var', async () => {
      process.env.SUDOCODE_DIR = '/env/.sudocode'
      await registry.load()

      const projectPath = '/Users/alex/repos/test-project'
      const customDir = '/custom/.sudocode'
      const projectInfo = registry.registerProject(projectPath, customDir)

      expect(projectInfo.sudocodeDir).toBe(customDir)
    })

    it('should update sudocodeDir for existing project when customSudocodeDir provided', async () => {
      delete process.env.SUDOCODE_DIR
      await registry.load()

      const projectPath = '/Users/alex/repos/test-project'
      
      // Register without custom dir first
      const project1 = registry.registerProject(projectPath)
      expect(project1.sudocodeDir).toBe(path.join(projectPath, '.sudocode'))

      // Re-register with custom dir - should NOT overwrite (sudocodeDir is write-once)
      const project2 = registry.registerProject(projectPath, '/new/custom/.sudocode')
      expect(project2.sudocodeDir).toBe(path.join(projectPath, '.sudocode'))
    })

    it('should NOT update sudocodeDir for existing project when SUDOCODE_DIR env changes', async () => {
      delete process.env.SUDOCODE_DIR
      await registry.load()

      const projectPath = '/Users/alex/repos/test-project'
      
      // Register without env var
      const project1 = registry.registerProject(projectPath)
      expect(project1.sudocodeDir).toBe(path.join(projectPath, '.sudocode'))

      // Set env var and re-register - should NOT overwrite (sudocodeDir is write-once)
      process.env.SUDOCODE_DIR = '/env/changed/.sudocode'
      const project2 = registry.registerProject(projectPath)
      expect(project2.sudocodeDir).toBe(path.join(projectPath, '.sudocode'))
    })

    it('should NOT update sudocodeDir when re-registering without override or env var', async () => {
      await registry.load()

      const projectPath = '/Users/alex/repos/test-project'
      
      // Register with env var
      process.env.SUDOCODE_DIR = '/initial/.sudocode'
      const project1 = registry.registerProject(projectPath)
      expect(project1.sudocodeDir).toBe('/initial/.sudocode')

      // Re-register without env var - should keep original (sudocodeDir is write-once)
      delete process.env.SUDOCODE_DIR
      const project2 = registry.registerProject(projectPath)
      expect(project2.sudocodeDir).toBe('/initial/.sudocode')
    })

    it('should handle SUDOCODE_DIR pointing outside project directory', async () => {
      process.env.SUDOCODE_DIR = '/completely/different/path/.sudocode'
      await registry.load()

      const projectPath = '/Users/alex/repos/test-project'
      const projectInfo = registry.registerProject(projectPath)

      expect(projectInfo.sudocodeDir).toBe('/completely/different/path/.sudocode')
      expect(projectInfo.path).toBe(projectPath)
      // sudocodeDir and project path are completely different
      expect(projectInfo.sudocodeDir).not.toContain(projectInfo.path)
    })

    it('should handle paths with spaces in customSudocodeDir', async () => {
      delete process.env.SUDOCODE_DIR
      await registry.load()

      const projectPath = '/Users/alex/repos/test-project'
      const customDir = '/path/with spaces/.sudocode'
      const projectInfo = registry.registerProject(projectPath, customDir)

      expect(projectInfo.sudocodeDir).toBe(customDir)
    })

    it('should persist sudocodeDir through save/load cycle', async () => {
      const testTempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'sudocode-sudodir-'))
      const testConfigPath = path.join(testTempDir, 'projects.json')
      const testRegistry = new ProjectRegistry(testConfigPath)

      try {
        await testRegistry.load()

        const projectPath = '/Users/alex/repos/test-project'
        const customDir = '/custom/.sudocode'
        testRegistry.registerProject(projectPath, customDir)

        await testRegistry.save()

        // Load in new instance
        const registry2 = new ProjectRegistry(testConfigPath)
        await registry2.load()

        const projects = registry2.getAllProjects()
        expect(projects).toHaveLength(1)
        expect(projects[0].sudocodeDir).toBe(customDir)
      } finally {
        if (fs.existsSync(testTempDir)) {
          fs.rmSync(testTempDir, { recursive: true, force: true })
        }
      }
    })
  })

  describe('getSudocodeDir', () => {
    beforeEach(async () => {
      delete process.env.SUDOCODE_DIR
      await registry.load()
    })

    afterEach(() => {
      delete process.env.SUDOCODE_DIR
    })

    it('should return stored sudocodeDir for registered project', async () => {
      const projectPath = '/Users/alex/repos/test-project'
      const customDir = '/custom/.sudocode'
      
      // Register with custom dir
      registry.registerProject(projectPath, customDir)
      
      // getSudocodeDir should return the stored value
      expect(registry.getSudocodeDir(projectPath)).toBe(customDir)
    })

    it('should return stored sudocodeDir even when SUDOCODE_DIR env var is set', async () => {
      const projectPath = '/Users/alex/repos/test-project'
      const customDir = '/custom/.sudocode'
      
      // Register with custom dir
      registry.registerProject(projectPath, customDir)
      
      // Set env var to something different
      process.env.SUDOCODE_DIR = '/env-var/.sudocode'
      
      // getSudocodeDir should still return the stored value (stored takes precedence)
      expect(registry.getSudocodeDir(projectPath)).toBe(customDir)
    })

    it('should return SUDOCODE_DIR env var for unregistered project', async () => {
      const projectPath = '/Users/alex/repos/unregistered-project'
      
      process.env.SUDOCODE_DIR = '/env-var/.sudocode'
      
      // Project is not registered, so env var should be used
      expect(registry.getSudocodeDir(projectPath)).toBe('/env-var/.sudocode')
    })

    it('should return default path for unregistered project without env var', async () => {
      const projectPath = '/Users/alex/repos/unregistered-project'
      
      // No env var, not registered - should return default
      expect(registry.getSudocodeDir(projectPath)).toBe(path.join(projectPath, '.sudocode'))
    })
  })
})
