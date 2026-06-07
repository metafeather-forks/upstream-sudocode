/**
 * Tests for AlignedFeedbackPanel component
 */

import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { BrowserRouter } from 'react-router-dom'
import { userEvent } from '@testing-library/user-event'
import { AlignedFeedbackPanel } from '@/components/specs/AlignedFeedbackPanel'
import type { IssueFeedback } from '@/types/api'

// Mock useProjectRoutes hook
vi.mock('@/hooks/useProjectRoutes', () => ({
  useProjectRoutes: () => ({
    paths: {
      issue: (id: string) => `/p/test-project/issues/${id}`,
      spec: (id: string) => `/p/test-project/specs/${id}`,
    },
    effectiveProjectId: 'test-project',
  }),
}))

// Wrapper component to provide router context
const Wrapper = ({ children }: { children: React.ReactNode }) => (
  <BrowserRouter>{children}</BrowserRouter>
)

describe('AlignedFeedbackPanel', () => {
  const createMockFeedback = (overrides: Partial<IssueFeedback> = {}): IssueFeedback => ({
    id: 'FB-001',
    from_id: 'ISSUE-001',
    from_uuid: 'uuid-issue-001',
    to_id: 'SPEC-001',
    to_uuid: 'uuid-spec-001',
    feedback_type: 'comment',
    content: 'Test feedback',
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    ...overrides,
  })

  it('should render empty state when no feedback provided', () => {
    render(
      <Wrapper>
        <AlignedFeedbackPanel feedback={[]} />
      </Wrapper>
    )

    // Should render the panel without any feedback cards
    expect(screen.queryByText('Test feedback')).not.toBeInTheDocument()
  })

  it('should render general comments (no anchor)', () => {
    const generalFeedback: IssueFeedback[] = [
      createMockFeedback({
        id: 'FB-001',
        content: 'General comment 1',
      }),
      createMockFeedback({
        id: 'FB-002',
        content: 'General comment 2',
        anchor: JSON.stringify({
          anchor_status: 'valid',
        }),
      }),
    ]

    render(
      <Wrapper>
        <AlignedFeedbackPanel feedback={generalFeedback} />
      </Wrapper>
    )

    expect(screen.getByText('General comment 1')).toBeInTheDocument()
    expect(screen.getByText('General comment 2')).toBeInTheDocument()
  })

  it('should render all anchored comments unconditionally', () => {
    const anchoredFeedback: IssueFeedback[] = [
      createMockFeedback({
        id: 'FB-001',
        content: 'Anchored feedback 1',
        anchor: JSON.stringify({
          line_number: 10,
          anchor_status: 'valid',
        }),
      }),
      createMockFeedback({
        id: 'FB-002',
        content: 'Anchored feedback 2',
        anchor: JSON.stringify({
          line_number: 20,
          anchor_status: 'valid',
        }),
      }),
    ]

    render(
      <Wrapper>
        <AlignedFeedbackPanel feedback={anchoredFeedback} />
      </Wrapper>
    )

    // Both should render regardless of positions
    expect(screen.getByText('Anchored feedback 1')).toBeInTheDocument()
    expect(screen.getByText('Anchored feedback 2')).toBeInTheDocument()
  })

  it('should show line number indicator for line-anchored feedback via FeedbackCard', () => {
    const feedback: IssueFeedback[] = [
      createMockFeedback({
        id: 'FB-001',
        content: 'Line feedback',
        anchor: JSON.stringify({
          line_number: 42,
          anchor_status: 'valid',
        }),
      }),
    ]

    render(
      <Wrapper>
        <AlignedFeedbackPanel feedback={feedback} />
      </Wrapper>
    )

    expect(screen.getByText('L42')).toBeInTheDocument()
  })

  it('should show section heading badge for heading-anchored feedback', () => {
    const feedback: IssueFeedback[] = [
      createMockFeedback({
        id: 'FB-001',
        content: 'Heading feedback',
        anchor: JSON.stringify({
          section_heading: 'Requirements',
          line_number: 5,
          anchor_status: 'valid',
        }),
      }),
    ]

    render(
      <Wrapper>
        <AlignedFeedbackPanel feedback={feedback} />
      </Wrapper>
    )

    expect(screen.getByText('§ Requirements')).toBeInTheDocument()
  })

  it('should call onScrollToHeading when heading badge is clicked', async () => {
    const user = userEvent.setup()
    const onScrollToHeading = vi.fn()
    const feedback: IssueFeedback[] = [
      createMockFeedback({
        id: 'FB-001',
        content: 'Heading feedback',
        anchor: JSON.stringify({
          section_heading: 'Requirements',
          line_number: 5,
          anchor_status: 'valid',
        }),
      }),
    ]

    render(
      <Wrapper>
        <AlignedFeedbackPanel feedback={feedback} onScrollToHeading={onScrollToHeading} />
      </Wrapper>
    )

    await user.click(screen.getByText('§ Requirements'))
    expect(onScrollToHeading).toHaveBeenCalledWith('Requirements')
  })

  it('should sort feedback by created_at', () => {
    const feedback: IssueFeedback[] = [
      createMockFeedback({
        id: 'FB-002',
        content: 'Second',
        created_at: '2024-01-02T00:00:00Z',
      }),
      createMockFeedback({
        id: 'FB-001',
        content: 'First',
        created_at: '2024-01-01T00:00:00Z',
      }),
      createMockFeedback({
        id: 'FB-003',
        content: 'Third',
        created_at: '2024-01-03T00:00:00Z',
      }),
    ]

    const { container } = render(
      <Wrapper>
        <AlignedFeedbackPanel feedback={feedback} />
      </Wrapper>
    )

    const cards = container.querySelectorAll('.rounded-lg')
    const texts = Array.from(cards).map((c) => c.textContent)
    const firstIdx = texts.findIndex((t) => t?.includes('First'))
    const secondIdx = texts.findIndex((t) => t?.includes('Second'))
    const thirdIdx = texts.findIndex((t) => t?.includes('Third'))

    expect(firstIdx).toBeLessThan(secondIdx)
    expect(secondIdx).toBeLessThan(thirdIdx)
  })

  it('should call onFeedbackClick when feedback is clicked', async () => {
    const user = userEvent.setup()
    const onFeedbackClick = vi.fn()
    const feedback: IssueFeedback[] = [
      createMockFeedback({
        id: 'FB-001',
        content: 'Clickable feedback',
      }),
    ]

    const { container } = render(
      <Wrapper>
        <AlignedFeedbackPanel feedback={feedback} onFeedbackClick={onFeedbackClick} />
      </Wrapper>
    )

    const feedbackCard = container.querySelector('.rounded-lg')
    expect(feedbackCard).not.toBeNull()
    await user.click(feedbackCard!)

    expect(onFeedbackClick).toHaveBeenCalledWith(feedback[0])
  })

  it('should handle invalid anchor JSON gracefully', () => {
    const feedback: IssueFeedback[] = [
      createMockFeedback({
        id: 'FB-001',
        content: 'Invalid anchor',
        anchor: 'invalid json',
      }),
    ]

    render(
      <Wrapper>
        <AlignedFeedbackPanel feedback={feedback} />
      </Wrapper>
    )

    // Should treat as general comment and still render
    expect(screen.getByText('Invalid anchor')).toBeInTheDocument()
  })

  it('should apply custom className', () => {
    const { container } = render(
      <Wrapper>
        <AlignedFeedbackPanel feedback={[]} className="custom-class" />
      </Wrapper>
    )

    const panel = container.querySelector('.custom-class')
    expect(panel).not.toBeNull()
  })

  it('should render all feedback items', () => {
    const feedback: IssueFeedback[] = [
      createMockFeedback({ id: 'FB-001', content: 'First' }),
      createMockFeedback({ id: 'FB-002', content: 'Second' }),
      createMockFeedback({ id: 'FB-003', content: 'Third' }),
    ]

    render(
      <Wrapper>
        <AlignedFeedbackPanel feedback={feedback} />
      </Wrapper>
    )

    expect(screen.getByText('First')).toBeInTheDocument()
    expect(screen.getByText('Second')).toBeInTheDocument()
    expect(screen.getByText('Third')).toBeInTheDocument()
  })

  it('should handle mixed general and anchored comments', () => {
    const mixedFeedback: IssueFeedback[] = [
      createMockFeedback({
        id: 'FB-001',
        content: 'General comment',
      }),
      createMockFeedback({
        id: 'FB-002',
        content: 'Anchored comment',
        anchor: JSON.stringify({
          line_number: 10,
          anchor_status: 'valid',
        }),
      }),
    ]

    render(
      <Wrapper>
        <AlignedFeedbackPanel feedback={mixedFeedback} />
      </Wrapper>
    )

    expect(screen.getByText('General comment')).toBeInTheDocument()
    expect(screen.getByText('Anchored comment')).toBeInTheDocument()
  })
})
