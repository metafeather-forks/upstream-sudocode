/**
 * Unit tests for getSudocodeDir utility function
 * Tests dynamic SUDOCODE_DIR resolution for server-side code
 */

import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import * as path from 'path'
import { getSudocodeDir } from '../../../src/utils/sudocode-dir.js'

describe('getSudocodeDir', () => {
  let originalSudocodeDir: string | undefined
  let originalCwd: string

  beforeEach(() => {
    originalSudocodeDir = process.env.SUDOCODE_DIR
    originalCwd = process.cwd()
  })

  afterEach(() => {
    // Restore original environment
    if (originalSudocodeDir !== undefined) {
      process.env.SUDOCODE_DIR = originalSudocodeDir
    } else {
      delete process.env.SUDOCODE_DIR
    }
  })

  describe('when SUDOCODE_DIR env var is set', () => {
    it('should return the SUDOCODE_DIR value', () => {
      process.env.SUDOCODE_DIR = '/custom/path/.sudocode'
      const result = getSudocodeDir()
      expect(result).toBe('/custom/path/.sudocode')
    })

    it('should return SUDOCODE_DIR even if it points outside cwd', () => {
      process.env.SUDOCODE_DIR = '/completely/different/location/.sudocode'
      const result = getSudocodeDir()
      expect(result).toBe('/completely/different/location/.sudocode')
    })

    it('should handle relative paths in SUDOCODE_DIR', () => {
      // Note: The implementation doesn't resolve relative paths,
      // so this tests current behavior
      process.env.SUDOCODE_DIR = '../relative/.sudocode'
      const result = getSudocodeDir()
      expect(result).toBe('../relative/.sudocode')
    })

    it('should handle paths with spaces', () => {
      process.env.SUDOCODE_DIR = '/path/with spaces/.sudocode'
      const result = getSudocodeDir()
      expect(result).toBe('/path/with spaces/.sudocode')
    })

    it('should handle paths with special characters', () => {
      process.env.SUDOCODE_DIR = '/path/with-dashes_and.dots/.sudocode'
      const result = getSudocodeDir()
      expect(result).toBe('/path/with-dashes_and.dots/.sudocode')
    })
  })

  describe('when SUDOCODE_DIR env var is NOT set', () => {
    it('should return <cwd>/.sudocode', () => {
      delete process.env.SUDOCODE_DIR
      const result = getSudocodeDir()
      expect(result).toBe(path.join(process.cwd(), '.sudocode'))
    })

    it('should reflect the current working directory', () => {
      delete process.env.SUDOCODE_DIR
      const result = getSudocodeDir()
      expect(result).toContain('.sudocode')
      expect(result.startsWith(process.cwd())).toBe(true)
    })
  })

  describe('edge cases', () => {
    it('should handle empty string SUDOCODE_DIR (treated as falsy)', () => {
      process.env.SUDOCODE_DIR = ''
      const result = getSudocodeDir()
      // Empty string is falsy, so should fall back to cwd
      expect(result).toBe(path.join(process.cwd(), '.sudocode'))
    })

    it('should return consistent results on repeated calls', () => {
      process.env.SUDOCODE_DIR = '/stable/path/.sudocode'
      const result1 = getSudocodeDir()
      const result2 = getSudocodeDir()
      const result3 = getSudocodeDir()
      expect(result1).toBe(result2)
      expect(result2).toBe(result3)
    })
  })
})
