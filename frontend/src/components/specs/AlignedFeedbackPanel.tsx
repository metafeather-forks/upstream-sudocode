import { useMemo, useState, useEffect, useRef } from 'react'
import { FeedbackCard } from './FeedbackCard'
import type {
  IssueFeedback,
  FeedbackAnchor,
  Relationship,
  EntityType,
  RelationshipType,
} from '@/types/api'
import { RelationshipList } from '@/components/relationships/RelationshipList'
import { RelationshipForm } from '@/components/relationships/RelationshipForm'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Plus, ChevronDown, ChevronUp } from 'lucide-react'

const RELATIONSHIPS_COLLAPSED_STORAGE_KEY = 'sudocode:specs:showRelationshipsCollapsed'

interface AlignedFeedbackPanelProps {
  feedback: IssueFeedback[]
  onFeedbackClick?: (feedback: IssueFeedback) => void
  onDismiss?: (id: string) => void
  onDelete?: (id: string) => void
  addFeedbackButton?: React.ReactNode
  className?: string
  relationships?: Relationship[]
  currentEntityId?: string
  currentEntityType?: EntityType
  onDeleteRelationship?: (relationship: Relationship) => void
  onCreateRelationship?: (
    toId: string,
    toType: EntityType,
    relationshipType: RelationshipType
  ) => void
  onScrollToHeading?: (heading: string) => void
  scrollToFeedbackId?: string | null
}

/**
 * Parse anchor from string to FeedbackAnchor object
 */
function parseAnchor(anchor: string | undefined): FeedbackAnchor | null {
  if (!anchor) return null
  try {
    return JSON.parse(anchor) as FeedbackAnchor
  } catch {
    return null
  }
}

/**
 * Feedback panel that displays all feedback in a vertical list sorted by creation date.
 *
 * - Each card with a section_heading anchor shows a clickable "§ Heading" badge
 * - Each card with only a line_number shows "L{n}" text
 * - General comments (no anchor) render without location indicator
 */
export function AlignedFeedbackPanel({
  feedback,
  onFeedbackClick,
  onDismiss,
  onDelete,
  addFeedbackButton,
  className = '',
  relationships,
  currentEntityId,
  currentEntityType = 'spec',
  onDeleteRelationship,
  onCreateRelationship,
  onScrollToHeading,
  scrollToFeedbackId,
}: AlignedFeedbackPanelProps) {
  const [showAddRelationship, setShowAddRelationship] = useState(false)
  const [isRelationshipsCollapsed, setIsRelationshipsCollapsed] = useState(() => {
    const stored = localStorage.getItem(RELATIONSHIPS_COLLAPSED_STORAGE_KEY)
    return stored !== null ? JSON.parse(stored) : false
  })

  const handleCreateRelationship = (
    toId: string,
    toType: EntityType,
    relationshipType: RelationshipType
  ) => {
    if (onCreateRelationship) {
      onCreateRelationship(toId, toType, relationshipType)
      setShowAddRelationship(false)
    }
  }

  // Save relationships collapsed preference to localStorage
  useEffect(() => {
    localStorage.setItem(
      RELATIONSHIPS_COLLAPSED_STORAGE_KEY,
      JSON.stringify(isRelationshipsCollapsed)
    )
  }, [isRelationshipsCollapsed])

  // Scroll to feedback card when scrollToFeedbackId changes
  const panelRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!scrollToFeedbackId) return
    const el = panelRef.current?.querySelector(
      `[data-feedback-id="${scrollToFeedbackId}"]`
    )
    if (el) {
      el.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
    }
  }, [scrollToFeedbackId])

  // Sort feedback by created_at
  const sortedFeedback = useMemo(() => {
    return [...feedback].sort(
      (a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime()
    )
  }, [feedback])

  return (
    <div
      ref={panelRef}
      className={`flex h-full w-64 flex-col bg-background pr-4 sm:w-80 md:w-96 lg:w-[28rem] xl:w-[30rem] 2xl:w-[40rem] ${className}`}
    >
      {/* Relationships Section */}
      {currentEntityId && (
        <div className="p-2">
          <div
            className="mb-2 flex cursor-pointer items-center justify-between rounded-md border p-2"
            onClick={() => setIsRelationshipsCollapsed(!isRelationshipsCollapsed)}
          >
            <div className="flex items-center gap-2">
              {isRelationshipsCollapsed ? (
                <ChevronDown className="h-4 w-4 text-muted-foreground" />
              ) : (
                <ChevronUp className="h-4 w-4 text-muted-foreground" />
              )}
              <h3 className="text-sm font-medium text-muted-foreground">Relationships</h3>
              {relationships && relationships.length > 0 && (
                <Badge variant="secondary">{relationships.length}</Badge>
              )}
            </div>
            {onCreateRelationship && (
              <Button
                variant="outline"
                size="xs"
                onClick={(e) => {
                  e.stopPropagation()
                  setShowAddRelationship(true)
                }}
                className="h-6"
              >
                <Plus className="mr-1 h-4 w-4" />
                Add
              </Button>
            )}
          </div>
          {!isRelationshipsCollapsed && relationships && relationships.length > 0 && (
            <RelationshipList
              relationships={relationships}
              currentEntityId={currentEntityId}
              currentEntityType={currentEntityType}
              onDelete={onDeleteRelationship}
              showEmpty={false}
              showGroupHeaders={false}
            />
          )}
        </div>
      )}

      {/* Add Relationship Dialog */}
      <Dialog open={showAddRelationship} onOpenChange={setShowAddRelationship}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Add Relationship</DialogTitle>
          </DialogHeader>
          {currentEntityId && (
            <RelationshipForm
              fromId={currentEntityId}
              fromType={currentEntityType}
              onSubmit={handleCreateRelationship}
              onCancel={() => setShowAddRelationship(false)}
            />
          )}
        </DialogContent>
      </Dialog>

      {/* Add Feedback Button */}
      {addFeedbackButton && <div className="p-2">{addFeedbackButton}</div>}

      {/* All feedback in a vertical list */}
      <div className="flex flex-1 flex-col gap-2 overflow-y-auto p-2">
        {sortedFeedback.map((fb) => {
          const anchor = parseAnchor(fb.anchor)

          return (
            <div key={fb.id} data-feedback-id={fb.id} className="w-full">
              {/* Location indicator */}
              {anchor?.section_heading && (
                <button
                  className="mb-1 inline-flex items-center rounded bg-muted px-1.5 py-0.5 text-xs font-medium text-muted-foreground hover:bg-accent hover:text-accent-foreground"
                  onClick={() => onScrollToHeading?.(anchor.section_heading!)}
                >
                  § {anchor.section_heading}
                </button>
              )}

              <FeedbackCard
                feedback={fb}
                onClick={() => onFeedbackClick?.(fb)}
                onDismiss={onDismiss ? () => onDismiss(fb.id) : undefined}
                onDelete={onDelete ? () => onDelete(fb.id) : undefined}
                maxHeight={800}
                isCompact={false}
              />
            </div>
          )
        })}
      </div>
    </div>
  )
}
