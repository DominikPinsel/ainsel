import { useState } from 'react'
import { ApiError } from '../../api/client'
import {
  usePersonaVersion,
  usePersonaVersions,
  useRollbackPersona,
  type PersonaVersionSummary,
} from '../../api/personas'
import { Button } from '../../primitives/Button'
import { ConfirmModal } from '../../primitives/ConfirmModal'
import { Markdown } from '../../primitives/Markdown'
import { Pager } from '../../primitives/Pager'
import { Panel } from '../../primitives/Panel'
import { RegisterTable, type Column } from '../../primitives/RegisterTable'
import { SectionStatus } from '../../primitives/SectionStatus'
import { Tag } from '../../primitives/Tag'
import { formatISO } from '../../utils/time'

const PAGE_SIZE_OPTIONS = [10, 20, 50] as const

// The version endpoints are gated on the persona's read/write access, so a
// caller who can open this page can still be refused the history. Say which
// happened instead of showing a bare "failed to load".
function describeError(err: unknown, fallback: string): string {
  if (err instanceof ApiError && err.status === 403) return 'Access denied.'
  if (err instanceof ApiError && err.status === 404) return 'Version not found.'
  return fallback
}

type PersonaVersionHistoryProps = {
  personaId: string
  /** The persona's live version, used to mark the current row. */
  currentVersion: number
}

/**
 * The persona's edit history: every stored version, newest first, with the
 * ability to read an old version's text and roll the persona back to it.
 */
export function PersonaVersionHistory({
  personaId,
  currentVersion,
}: PersonaVersionHistoryProps) {
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [viewing, setViewing] = useState<number | null>(null)
  const [rollbackTo, setRollbackTo] = useState<number | null>(null)

  const versions = usePersonaVersions(personaId, { page, pageSize })
  const version = usePersonaVersion(personaId, viewing)
  const rollback = useRollbackPersona()

  const columns: readonly Column<PersonaVersionSummary>[] = [
    {
      key: 'version',
      header: 'Version',
      width: 160,
      cell: (v) => (
        <span style={{ display: 'inline-flex', gap: 6, alignItems: 'center' }}>
          <Tag>{`v${v.versionNumber}`}</Tag>
          {v.versionNumber === currentVersion ? <Tag variant="ok">current</Tag> : null}
        </span>
      ),
    },
    {
      key: 'created',
      header: 'Created',
      cell: (v) => <span className="num">{formatISO(v.createdAt)}</span>,
    },
    {
      key: 'actions',
      header: '',
      width: 220,
      align: 'right',
      cell: (v) => (
        <span style={{ display: 'inline-flex', gap: 8 }}>
          <Button
            size="sm"
            aria-label={`View version ${v.versionNumber}`}
            onClick={() => setViewing(viewing === v.versionNumber ? null : v.versionNumber)}
          >
            {viewing === v.versionNumber ? 'Hide' : 'View'}
          </Button>
          <Button
            size="sm"
            aria-label={`Roll back to version ${v.versionNumber}`}
            // Rolling the current version back onto itself is a no-op that
            // would still burn a version number.
            disabled={v.versionNumber === currentVersion}
            onClick={() => setRollbackTo(v.versionNumber)}
          >
            Roll back
          </Button>
        </span>
      ),
    },
  ]

  const onRollback = async () => {
    if (rollbackTo === null) return
    try {
      await rollback.mutateAsync({ id: personaId, toVersion: rollbackTo })
      setRollbackTo(null)
      setViewing(null)
    } catch {
      // Leave the modal open; rollback.error carries the reason.
    }
  }

  return (
    <div style={{ display: 'grid', gap: 24 }}>
      <Panel title="Version history" className="cropped">
        {versions.isLoading ? <SectionStatus state="loading" /> : null}
        {versions.error ? (
          <SectionStatus
            state="error"
            title={describeError(versions.error, 'Failed to load version history.')}
            onRetry={() => versions.refetch()}
          />
        ) : null}
        {versions.data ? (
          <>
            <RegisterTable
              rows={versions.data.items}
              columns={columns}
              rowKey={(v) => String(v.versionNumber)}
              withRowNumbers={false}
              emptyLabel="No versions stored yet."
            />
            <Pager
              page={page}
              pageSize={pageSize}
              total={versions.data.total}
              pageSizeOptions={PAGE_SIZE_OPTIONS}
              onPageChange={setPage}
              onPageSizeChange={(n) => {
                setPageSize(n)
                setPage(1)
              }}
            />
          </>
        ) : null}
      </Panel>

      {viewing !== null ? (
        <Panel
          title={`Version ${viewing}`}
          className="cropped"
          right={
            <Button size="sm" onClick={() => setViewing(null)}>
              Close
            </Button>
          }
        >
          {version.isLoading ? <SectionStatus state="loading" /> : null}
          {version.error ? (
            <SectionStatus
              state="error"
              title={describeError(version.error, 'Failed to load this version.')}
              onRetry={() => version.refetch()}
            />
          ) : null}
          {version.data ? <Markdown source={version.data.text} /> : null}
        </Panel>
      ) : null}

      <ConfirmModal
        open={rollbackTo !== null}
        title={`Roll back to version ${rollbackTo ?? ''}?`}
        destructive
        confirmLabel={rollback.isPending ? 'Rolling back…' : 'Roll back'}
        error={rollback.error ? describeError(rollback.error, rollback.error.message) : null}
        body={
          <p>
            Copies the text of version {rollbackTo ?? ''} into a new version (v
            {currentVersion + 1}) and makes it current. Existing history is
            kept — nothing is deleted, and agents pick up the new text on their
            next start.
          </p>
        }
        onConfirm={onRollback}
        onCancel={() => {
          setRollbackTo(null)
          rollback.reset()
        }}
      />
    </div>
  )
}
