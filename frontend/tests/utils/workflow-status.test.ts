import { describe, it, expect } from 'vitest'
import {
  deriveStepStatusFromIssue,
  areDependenciesMet,
  deriveWorkflowStepStatuses,
} from '@/utils/workflow-status'
import type { WorkflowStepStatus } from '@/types/workflow'

describe('workflow-status', () => {
  describe('deriveStepStatusFromIssue', () => {
    it('should return completed when issue is closed', () => {
      expect(deriveStepStatusFromIssue('closed', 'pending', true)).toBe('completed')
      expect(deriveStepStatusFromIssue('closed', 'ready', true)).toBe('completed')
      expect(deriveStepStatusFromIssue('closed', 'running', true)).toBe('completed')
    })

    it('should return running when issue is in_progress', () => {
      expect(deriveStepStatusFromIssue('in_progress', 'pending', true)).toBe('running')
      expect(deriveStepStatusFromIssue('in_progress', 'ready', true)).toBe('running')
    })

    it('should return running when issue needs_review', () => {
      expect(deriveStepStatusFromIssue('needs_review', 'pending', true)).toBe('running')
      expect(deriveStepStatusFromIssue('needs_review', 'ready', true)).toBe('running')
    })

    it('should return blocked when issue is blocked', () => {
      expect(deriveStepStatusFromIssue('blocked', 'ready', true)).toBe('blocked')
      expect(deriveStepStatusFromIssue('blocked', 'pending', true)).toBe('blocked')
    })

    it('should return ready when issue is open and deps met', () => {
      expect(deriveStepStatusFromIssue('open', 'pending', true)).toBe('ready')
    })

    it('should return pending when issue is open and deps not met', () => {
      expect(deriveStepStatusFromIssue('open', 'pending', false)).toBe('pending')
      expect(deriveStepStatusFromIssue('open', 'ready', false)).toBe('pending')
    })

    it('should preserve failed status regardless of issue status', () => {
      expect(deriveStepStatusFromIssue('closed', 'failed', true)).toBe('failed')
      expect(deriveStepStatusFromIssue('open', 'failed', true)).toBe('failed')
      expect(deriveStepStatusFromIssue('in_progress', 'failed', true)).toBe('failed')
    })

    it('should preserve skipped status regardless of issue status', () => {
      expect(deriveStepStatusFromIssue('closed', 'skipped', true)).toBe('skipped')
      expect(deriveStepStatusFromIssue('in_progress', 'skipped', true)).toBe('skipped')
      expect(deriveStepStatusFromIssue('open', 'skipped', false)).toBe('skipped')
    })
  })

  describe('areDependenciesMet', () => {
    const steps: { id: string; status: WorkflowStepStatus }[] = [
      { id: 'step-1', status: 'completed' },
      { id: 'step-2', status: 'running' },
      { id: 'step-3', status: 'pending' },
    ]

    it('should return true when no dependencies', () => {
      expect(areDependenciesMet([], steps)).toBe(true)
    })

    it('should return true when all deps completed', () => {
      expect(areDependenciesMet(['step-1'], steps)).toBe(true)
    })

    it('should return false when any dep not completed', () => {
      expect(areDependenciesMet(['step-1', 'step-2'], steps)).toBe(false)
      expect(areDependenciesMet(['step-2'], steps)).toBe(false)
      expect(areDependenciesMet(['step-3'], steps)).toBe(false)
    })

    it('should return false when dep does not exist', () => {
      expect(areDependenciesMet(['non-existent'], steps)).toBe(false)
    })
  })

  describe('deriveWorkflowStepStatuses', () => {
    it('should return original steps array when no issues map provided', () => {
      const steps = [
        { id: 'step-1', issueId: 'i-001', status: 'pending' as const, index: 0, dependencies: [] },
      ]
      const result = deriveWorkflowStepStatuses(steps, {})
      expect(result).toBe(steps) // Same reference
    })

    it('should derive step status from issue status', () => {
      const steps = [
        { id: 'step-1', issueId: 'i-001', status: 'pending' as const, index: 0, dependencies: [] },
      ]
      const issues = {
        'i-001': { status: 'closed' as const },
      }
      const result = deriveWorkflowStepStatuses(steps, issues)
      expect(result[0].status).toBe('completed')
    })

    it('should handle dependency chain correctly', () => {
      const steps = [
        { id: 'step-1', issueId: 'i-001', status: 'pending' as const, index: 0, dependencies: [] },
        {
          id: 'step-2',
          issueId: 'i-002',
          status: 'pending' as const,
          index: 1,
          dependencies: ['step-1'],
        },
      ]
      const issues = {
        'i-001': { status: 'open' as const },
        'i-002': { status: 'open' as const },
      }
      const result = deriveWorkflowStepStatuses(steps, issues)
      // step-1 has no deps, so it becomes ready
      expect(result[0].status).toBe('ready')
      // step-2 deps on step-1 which is not completed, so stays pending
      expect(result[1].status).toBe('pending')
    })

    it('should preserve failed steps even if issue is closed', () => {
      const steps = [
        { id: 'step-1', issueId: 'i-001', status: 'failed' as const, index: 0, dependencies: [] },
      ]
      const issues = {
        'i-001': { status: 'closed' as const },
      }
      const result = deriveWorkflowStepStatuses(steps, issues)
      expect(result[0].status).toBe('failed')
      expect(result).toBe(steps) // Same reference since no change
    })

    it('should preserve skipped steps', () => {
      const steps = [
        { id: 'step-1', issueId: 'i-001', status: 'skipped' as const, index: 0, dependencies: [] },
      ]
      const issues = {
        'i-001': { status: 'in_progress' as const },
      }
      const result = deriveWorkflowStepStatuses(steps, issues)
      expect(result[0].status).toBe('skipped')
      expect(result).toBe(steps) // Same reference since no change
    })

    it('should return same array reference when no changes', () => {
      const steps = [
        { id: 'step-1', issueId: 'i-001', status: 'completed' as const, index: 0, dependencies: [] },
      ]
      const issues = {
        'i-001': { status: 'closed' as const },
      }
      const result = deriveWorkflowStepStatuses(steps, issues)
      expect(result).toBe(steps) // Same reference
    })

    it('should handle mixed statuses correctly', () => {
      const steps = [
        { id: 'step-1', issueId: 'i-001', status: 'completed' as const, index: 0, dependencies: [] },
        { id: 'step-2', issueId: 'i-002', status: 'pending' as const, index: 1, dependencies: ['step-1'] },
        { id: 'step-3', issueId: 'i-003', status: 'failed' as const, index: 2, dependencies: ['step-2'] },
      ]
      const issues = {
        'i-001': { status: 'closed' as const },
        'i-002': { status: 'in_progress' as const },
        'i-003': { status: 'open' as const },
      }
      const result = deriveWorkflowStepStatuses(steps, issues)
      expect(result[0].status).toBe('completed') // matches issue
      expect(result[1].status).toBe('running') // derived from in_progress
      expect(result[2].status).toBe('failed') // preserved
    })
  })
})
