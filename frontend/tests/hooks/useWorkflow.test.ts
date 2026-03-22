import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useWorkflow } from '@/hooks/useWorkflows'
import { workflowsApi, issuesApi } from '@/lib/api'
import type { Workflow, WorkflowStep } from '@/types/workflow'
import type { Issue, WebSocketMessage } from '@/types/api'
import { createElement, type ReactNode } from 'react'

// Track message handlers for simulating WebSocket messages
let messageHandlers: Map<string, (message: WebSocketMessage) => void>
let mockSubscriptions: Set<string>

// Mock Project context
let mockProjectId: string | null = 'test-project-id'

vi.mock('@/lib/api', () => ({
  workflowsApi: {
    get: vi.fn(),
  },
  issuesApi: {
    getAll: vi.fn(),
  },
}))

// Mock WebSocket context
vi.mock('@/contexts/WebSocketContext', () => ({
  useWebSocketContext: () => ({
    connected: true,
    subscribe: vi.fn((channel: string) => {
      mockSubscriptions.add(channel)
    }),
    unsubscribe: vi.fn((channel: string) => {
      mockSubscriptions.delete(channel)
    }),
    addMessageHandler: vi.fn((id: string, handler: (message: WebSocketMessage) => void) => {
      messageHandlers.set(id, handler)
    }),
    removeMessageHandler: vi.fn((id: string) => {
      messageHandlers.delete(id)
    }),
    lastMessage: null,
  }),
}))

vi.mock('@/hooks/useProject', () => ({
  useProject: () => ({
    currentProjectId: mockProjectId,
    setCurrentProjectId: vi.fn(),
    currentProject: null,
    setCurrentProject: vi.fn(),
    clearProject: vi.fn(),
  }),
}))

const mockSteps: WorkflowStep[] = [
  {
    id: 'step-1',
    issueId: 'i-abc1',
    status: 'completed',
    index: 0,
    dependencies: [],
    executionId: 'exec-1',
  },
  {
    id: 'step-2',
    issueId: 'i-abc2',
    status: 'running',
    index: 1,
    dependencies: ['step-1'],
  },
]

const mockWorkflow: Workflow = {
  id: 'wf-001',
  title: 'Test Workflow',
  source: {
    type: 'goal',
    goal: 'Test goal',
  },
  status: 'running',
  steps: mockSteps,
  baseBranch: 'main',
  currentStepIndex: 1,
  config: {
    engineType: 'sequential',
    parallelism: 'sequential',
    onFailure: 'pause',
    autoCommitAfterStep: true,
    defaultAgentType: 'claude-code',
    autonomyLevel: 'human_in_the_loop',
  },
  createdAt: '2025-01-15T09:00:00Z',
  updatedAt: '2025-01-15T10:05:00Z',
}

const mockIssues: Issue[] = [
  {
    id: 'i-abc1',
    uuid: 'uuid-1',
    title: 'First Issue',
    content: 'Content 1',
    status: 'closed',
    priority: 1,
    assignee: undefined,
    created_at: '2024-01-01',
    updated_at: '2024-01-01',
    closed_at: undefined,
    parent_id: undefined,
  },
  {
    id: 'i-abc2',
    uuid: 'uuid-2',
    title: 'Second Issue',
    content: 'Content 2',
    status: 'in_progress',
    priority: 2,
    assignee: undefined,
    created_at: '2024-01-02',
    updated_at: '2024-01-02',
    closed_at: undefined,
    parent_id: undefined,
  },
  {
    id: 'i-archived',
    uuid: 'uuid-3',
    title: 'Archived Issue',
    content: 'Content 3',
    status: 'closed',
    priority: 3,
    assignee: undefined,
    created_at: '2024-01-03',
    updated_at: '2024-01-03',
    closed_at: undefined,
    parent_id: undefined,
    archived: true,
  },
]

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        gcTime: 0,
      },
    },
  })

  return function Wrapper({ children }: { children: ReactNode }) {
    return createElement(QueryClientProvider, { client: queryClient }, children)
  }
}

describe('useWorkflow', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockProjectId = 'test-project-id'
    messageHandlers = new Map()
    mockSubscriptions = new Set()
  })

  afterEach(() => {
    messageHandlers.clear()
    mockSubscriptions.clear()
  })

  describe('query key with projectId', () => {
    it('should use workflow-issues query key with currentProjectId', async () => {
      vi.mocked(workflowsApi.get).mockResolvedValue(mockWorkflow)
      vi.mocked(issuesApi.getAll).mockResolvedValue(mockIssues)

      const { result } = renderHook(() => useWorkflow('wf-001'), {
        wrapper: createWrapper(),
      })

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false)
      })

      // Verify workflow was fetched
      expect(workflowsApi.get).toHaveBeenCalledWith('wf-001')
      // Verify issues were fetched
      expect(issuesApi.getAll).toHaveBeenCalled()
    })

    it('should not fetch issues when projectId is null', async () => {
      mockProjectId = null
      vi.mocked(workflowsApi.get).mockResolvedValue(mockWorkflow)

      const { result } = renderHook(() => useWorkflow('wf-001'), {
        wrapper: createWrapper(),
      })

      await waitFor(() => {
        expect(result.current.workflow).toBeDefined()
      })

      // Issues should NOT be fetched when projectId is null
      expect(issuesApi.getAll).not.toHaveBeenCalled()
    })

    it('should share cache between multiple workflows in the same project', async () => {
      vi.mocked(workflowsApi.get).mockResolvedValue(mockWorkflow)
      vi.mocked(issuesApi.getAll).mockResolvedValue(mockIssues)

      const queryClient = new QueryClient({
        defaultOptions: {
          queries: {
            retry: false,
            gcTime: Infinity,
            staleTime: Infinity,
          },
        },
      })

      const wrapper = ({ children }: { children: ReactNode }) =>
        createElement(QueryClientProvider, { client: queryClient }, children)

      // Render first workflow hook
      const { result: result1 } = renderHook(() => useWorkflow('wf-001'), { wrapper })

      await waitFor(() => {
        expect(result1.current.isLoading).toBe(false)
      })

      expect(issuesApi.getAll).toHaveBeenCalledTimes(1)

      // Render second workflow hook (should use cached issues)
      const { result: result2 } = renderHook(() => useWorkflow('wf-002'), { wrapper })

      await waitFor(() => {
        expect(result2.current.issues).toBeDefined()
      })

      // Issues API should still only be called once (using cache)
      expect(issuesApi.getAll).toHaveBeenCalledTimes(1)
    })
  })

  describe('WebSocket subscription', () => {
    it('should subscribe to both workflow and issue channels', async () => {
      vi.mocked(workflowsApi.get).mockResolvedValue(mockWorkflow)
      vi.mocked(issuesApi.getAll).mockResolvedValue(mockIssues)

      renderHook(() => useWorkflow('wf-001'), {
        wrapper: createWrapper(),
      })

      await waitFor(() => {
        expect(mockSubscriptions.has('workflow')).toBe(true)
        expect(mockSubscriptions.has('issue')).toBe(true)
      })
    })

    it('should unsubscribe from issue channel on cleanup', async () => {
      vi.mocked(workflowsApi.get).mockResolvedValue(mockWorkflow)
      vi.mocked(issuesApi.getAll).mockResolvedValue(mockIssues)

      const { unmount } = renderHook(() => useWorkflow('wf-001'), {
        wrapper: createWrapper(),
      })

      await waitFor(() => {
        expect(mockSubscriptions.has('issue')).toBe(true)
      })

      unmount()

      // After unmount, issue channel should be unsubscribed
      expect(mockSubscriptions.has('issue')).toBe(false)
    })

    it('should register message handler with workflow-specific id', async () => {
      vi.mocked(workflowsApi.get).mockResolvedValue(mockWorkflow)
      vi.mocked(issuesApi.getAll).mockResolvedValue(mockIssues)

      renderHook(() => useWorkflow('wf-001'), {
        wrapper: createWrapper(),
      })

      await waitFor(() => {
        expect(messageHandlers.has('workflow-detail-wf-001')).toBe(true)
      })
    })
  })

  describe('issue event handling', () => {
    it('should invalidate workflow-issues cache on issue_updated event', async () => {
      vi.mocked(workflowsApi.get).mockResolvedValue(mockWorkflow)
      vi.mocked(issuesApi.getAll).mockResolvedValue(mockIssues)

      const queryClient = new QueryClient({
        defaultOptions: {
          queries: {
            retry: false,
            gcTime: 0,
          },
        },
      })
      const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

      const wrapper = ({ children }: { children: ReactNode }) =>
        createElement(QueryClientProvider, { client: queryClient }, children)

      renderHook(() => useWorkflow('wf-001'), { wrapper })

      await waitFor(() => {
        expect(messageHandlers.has('workflow-detail-wf-001')).toBe(true)
      })

      // Simulate issue_updated WebSocket message
      const handler = messageHandlers.get('workflow-detail-wf-001')!
      act(() => {
        handler({
          type: 'issue_updated',
          data: { id: 'i-abc1' },
        } as WebSocketMessage)
      })

      // Verify invalidation was called with workflow-issues key
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ['workflow-issues', 'test-project-id'],
      })
    })

    it('should invalidate workflow-issues cache on issue_created event', async () => {
      vi.mocked(workflowsApi.get).mockResolvedValue(mockWorkflow)
      vi.mocked(issuesApi.getAll).mockResolvedValue(mockIssues)

      const queryClient = new QueryClient({
        defaultOptions: {
          queries: {
            retry: false,
            gcTime: 0,
          },
        },
      })
      const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

      const wrapper = ({ children }: { children: ReactNode }) =>
        createElement(QueryClientProvider, { client: queryClient }, children)

      renderHook(() => useWorkflow('wf-001'), { wrapper })

      await waitFor(() => {
        expect(messageHandlers.has('workflow-detail-wf-001')).toBe(true)
      })

      // Simulate issue_created WebSocket message
      const handler = messageHandlers.get('workflow-detail-wf-001')!
      act(() => {
        handler({
          type: 'issue_created',
          data: { id: 'i-new' },
        } as WebSocketMessage)
      })

      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ['workflow-issues', 'test-project-id'],
      })
    })

    it('should invalidate workflow-issues cache on issue_deleted event', async () => {
      vi.mocked(workflowsApi.get).mockResolvedValue(mockWorkflow)
      vi.mocked(issuesApi.getAll).mockResolvedValue(mockIssues)

      const queryClient = new QueryClient({
        defaultOptions: {
          queries: {
            retry: false,
            gcTime: 0,
          },
        },
      })
      const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

      const wrapper = ({ children }: { children: ReactNode }) =>
        createElement(QueryClientProvider, { client: queryClient }, children)

      renderHook(() => useWorkflow('wf-001'), { wrapper })

      await waitFor(() => {
        expect(messageHandlers.has('workflow-detail-wf-001')).toBe(true)
      })

      // Simulate issue_deleted WebSocket message
      const handler = messageHandlers.get('workflow-detail-wf-001')!
      act(() => {
        handler({
          type: 'issue_deleted',
          data: { id: 'i-abc1' },
        } as WebSocketMessage)
      })

      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ['workflow-issues', 'test-project-id'],
      })
    })
  })

  describe('issue data enrichment', () => {
    it('should build issues map from fetched issues', async () => {
      vi.mocked(workflowsApi.get).mockResolvedValue(mockWorkflow)
      vi.mocked(issuesApi.getAll).mockResolvedValue(mockIssues)

      const { result } = renderHook(() => useWorkflow('wf-001'), {
        wrapper: createWrapper(),
      })

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false)
      })

      // Should have issues map with only the issues referenced in workflow steps
      expect(result.current.issues).toBeDefined()
      expect(result.current.issues!['i-abc1']).toBeDefined()
      expect(result.current.issues!['i-abc2']).toBeDefined()
      // Should not include issues not in workflow steps
      expect(result.current.issues!['i-archived']).toBeUndefined()
    })

    it('should include archived issues in the issues map when they are in workflow steps', async () => {
      const workflowWithArchivedStep: Workflow = {
        ...mockWorkflow,
        steps: [
          ...mockSteps,
          {
            id: 'step-3',
            issueId: 'i-archived',
            status: 'pending',
            index: 2,
            dependencies: ['step-2'],
          },
        ],
      }

      vi.mocked(workflowsApi.get).mockResolvedValue(workflowWithArchivedStep)
      vi.mocked(issuesApi.getAll).mockResolvedValue(mockIssues)

      const { result } = renderHook(() => useWorkflow('wf-001'), {
        wrapper: createWrapper(),
      })

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false)
      })

      // Should include the archived issue since it's referenced in a workflow step
      expect(result.current.issues!['i-archived']).toBeDefined()
      expect(result.current.issues!['i-archived'].title).toBe('Archived Issue')
    })
  })

  describe('workflow event handling', () => {
    it('should only invalidate workflow query for matching workflow events', async () => {
      vi.mocked(workflowsApi.get).mockResolvedValue(mockWorkflow)
      vi.mocked(issuesApi.getAll).mockResolvedValue(mockIssues)

      const queryClient = new QueryClient({
        defaultOptions: {
          queries: {
            retry: false,
            gcTime: 0,
          },
        },
      })
      const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

      const wrapper = ({ children }: { children: ReactNode }) =>
        createElement(QueryClientProvider, { client: queryClient }, children)

      renderHook(() => useWorkflow('wf-001'), { wrapper })

      await waitFor(() => {
        expect(messageHandlers.has('workflow-detail-wf-001')).toBe(true)
      })

      // Clear previous invalidation calls (from initial fetch)
      invalidateSpy.mockClear()

      // Simulate workflow_updated for a DIFFERENT workflow
      const handler = messageHandlers.get('workflow-detail-wf-001')!
      act(() => {
        handler({
          type: 'workflow_updated',
          data: { id: 'wf-other' },
        } as WebSocketMessage)
      })

      // Should NOT invalidate workflow detail (wrong workflow ID)
      expect(invalidateSpy).not.toHaveBeenCalledWith(
        expect.objectContaining({
          queryKey: expect.arrayContaining(['workflows', 'detail', 'wf-001']),
        })
      )

      // Now simulate workflow_updated for THIS workflow
      act(() => {
        handler({
          type: 'workflow_updated',
          data: { id: 'wf-001' },
        } as WebSocketMessage)
      })

      // Should invalidate workflow detail
      expect(invalidateSpy).toHaveBeenCalledWith(
        expect.objectContaining({
          queryKey: expect.arrayContaining(['workflows', 'detail', 'wf-001']),
        })
      )
    })
  })
})
