/**
 * Workflow Step Status Derivation Utilities
 *
 * Provides functions to derive workflow step status from issue status,
 * enabling the UI to reflect issue changes made outside of workflow execution.
 *
 * @see docs/workflow-step-status-sync-plan.md for design details
 */

import type { IssueStatus } from '@/types/api'
import type { WorkflowStep, WorkflowStepStatus } from '@/types/workflow'

/**
 * Derives the display status for a workflow step based on the current issue status.
 *
 * This allows workflow step status to reflect changes made to issues outside
 * of the workflow execution context (e.g., manual updates, CLI changes, integrations).
 *
 * Preserves workflow engine terminal states (failed, skipped) since these
 * represent execution outcomes that cannot be inferred from issue status alone.
 *
 * @param issueStatus - Current status of the issue
 * @param originalStepStatus - The step's status from the workflow data
 * @param dependenciesMet - Whether all dependencies are completed
 * @returns The derived step status for display
 */
export function deriveStepStatusFromIssue(
  issueStatus: IssueStatus,
  originalStepStatus: WorkflowStepStatus,
  dependenciesMet: boolean
): WorkflowStepStatus {
  // Preserve workflow engine terminal states - these represent execution outcomes
  // that cannot be inferred from issue status alone
  if (originalStepStatus === 'failed' || originalStepStatus === 'skipped') {
    return originalStepStatus
  }

  // Map issue status to step status
  switch (issueStatus) {
    case 'closed':
      return 'completed'
    case 'in_progress':
      return 'running'
    case 'needs_review':
      // needs_review means work is done but awaiting review - still "active" in workflow terms
      return 'running'
    case 'blocked':
      return 'blocked'
    case 'open':
      // Open issues are either ready (can execute) or pending (waiting on deps)
      return dependenciesMet ? 'ready' : 'pending'
    default:
      // Fallback to original status if issue status is unexpected
      return originalStepStatus
  }
}

/**
 * Checks if all dependencies for a step are completed.
 *
 * @param stepDependencies - Array of step IDs this step depends on
 * @param allSteps - All steps in the workflow (with their current statuses)
 * @returns true if all dependencies are completed
 */
export function areDependenciesMet(
  stepDependencies: string[],
  allSteps: Pick<WorkflowStep, 'id' | 'status'>[]
): boolean {
  if (stepDependencies.length === 0) return true

  return stepDependencies.every((depId) => {
    const depStep = allSteps.find((s) => s.id === depId)
    return depStep?.status === 'completed'
  })
}

/**
 * Derives step statuses for all steps in a workflow based on current issue statuses.
 *
 * This function is designed to be called from the useWorkflow hook to compute
 * derived statuses at render time.
 *
 * @param steps - The workflow steps with original statuses
 * @param issues - Map of issue ID to Issue object
 * @returns Steps with derived statuses (same array reference if no changes)
 */
export function deriveWorkflowStepStatuses<
  T extends WorkflowStep,
  I extends { status: IssueStatus }
>(steps: T[], issues: Record<string, I>): T[] {
  let hasChanges = false

  const derivedSteps = steps.map((step) => {
    const issue = issues[step.issueId]
    if (!issue) return step // No issue data, keep original status

    // Check if dependencies are met (using original statuses for dep check)
    const dependenciesMet = areDependenciesMet(step.dependencies, steps)

    const derivedStatus = deriveStepStatusFromIssue(
      issue.status,
      step.status,
      dependenciesMet
    )

    // Only create new object if status changed
    if (derivedStatus === step.status) return step

    hasChanges = true
    return { ...step, status: derivedStatus }
  })

  // Return original array reference if no changes (helps with React memo)
  return hasChanges ? derivedSteps : steps
}
