import { Trans, useLingui } from "@lingui/react/macro"
import { useState } from "react"
import { Link, useNavigate } from "react-router"

import { OwnDomainCard } from "@/features/settings/OwnDomainCard"
import { CopyButton, ErrorLine, Segmented, toast, useConfirm } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { useAnalyticsProperties } from "@/features/analytics/properties"
import { useInboxChannels } from "@/features/broadcasts/queries"
import { useDeleteBoard, useUpdateBoard } from "@/features/roadmap/queries"
import { Card, ChoiceSelect, Field, FormCard, Section, ToggleRow } from "@/features/settings/ui"
import { validSlug } from "@/features/status/CreatePageDialog"
import type { RoadmapBoard, RoadmapBoardUpdate, RoadmapSuggestions } from "@/lib/api"

function Visibility({ board }: { board: RoadmapBoard }) {
  const patch = useUpdateBoard(board.id)
  return (
    <Section title={<Trans>Visibility</Trans>}>
      <Card>
        <ToggleRow
          title={<Trans>Public</Trans>}
          hint={
            <Trans>
              On: anyone with the address reads the board, and signed-in readers vote and suggest ideas. Off: only your
              team sees it, as a tool to sort feedback.
            </Trans>
          }
        >
          <Switch
            checked={board.public}
            disabled={patch.isPending}
            onCheckedChange={(v) => patch.mutate({ public: v })}
            data-testid="board-public"
          />
        </ToggleRow>
        <div className="flex flex-wrap items-center gap-3">
          {board.public ? (
            <a
              href={board.url}
              target="_blank"
              rel="noreferrer noopener"
              className="min-w-0 flex-1 break-all text-body text-brand underline-offset-4 hover:underline"
              data-testid="board-public-url"
            >
              {board.url}
            </a>
          ) : (
            <span className="min-w-0 flex-1 break-all text-body text-muted-foreground" data-testid="board-public-url">
              {board.url}
            </span>
          )}
          <CopyButton value={board.url} />
        </div>
        {!board.public && (
          <p className="text-small text-faint">
            <Trans>The address answers “not found” until you make the board public.</Trans>
          </p>
        )}
        <ErrorLine error={patch.error} />
      </Card>
    </Section>
  )
}

function General({ board }: { board: RoadmapBoard }) {
  const { t } = useLingui()
  const patch = useUpdateBoard(board.id)
  const channels = (useInboxChannels(board.inbox_id).data ?? []).filter((c) => c.kind === "chat")
  const properties = useAnalyticsProperties().data?.items ?? []
  const [name, setName] = useState(board.name)
  const [description, setDescription] = useState(board.description)
  const [slug, setSlug] = useState(board.slug)
  const [suggestions, setSuggestions] = useState<RoadmapSuggestions>(board.suggestions)
  const [channel, setChannel] = useState(board.channel_id)
  const [property, setProperty] = useState(board.analytics_property_id ?? "")

  const body: RoadmapBoardUpdate = {}
  if (name.trim() !== board.name) body.name = name.trim()
  if (description.trim() !== board.description) body.description = description.trim()
  if (slug !== board.slug) body.slug = slug
  if (suggestions !== board.suggestions) body.suggestions = suggestions
  if (channel !== board.channel_id) body.channel_id = channel
  if ((property || null) !== (board.analytics_property_id ?? null)) body.analytics_property_id = property || null
  const dirty = Object.keys(body).length > 0
  const ok = !!name.trim() && validSlug(slug)
  const address = board.url.endsWith(board.slug)
    ? board.url.slice(0, board.url.length - board.slug.length) + slug
    : board.url
  const modes: [RoadmapSuggestions, string][] = [
    ["moderated", t`Moderated`],
    ["open", t`Open`],
    ["off", t`Off`],
  ]
  const modeHint: Record<RoadmapSuggestions, React.ReactNode> = {
    off: <Trans>Readers cannot suggest ideas. Votes and replies still work.</Trans>,
    moderated: (
      <Trans>
        A reader's posts wait for a member until one of theirs is approved. Signed-in users of your app publish at once.
      </Trans>
    ),
    open: <Trans>Everyone's posts are published at once, except those of authors you marked as spam.</Trans>,
  }

  return (
    <Section title={<Trans>Board</Trans>}>
      <FormCard
        onSubmit={() =>
          ok &&
          patch.mutate(body, {
            onSuccess: (saved) => {
              setName(saved.name)
              setSlug(saved.slug)
              toast(t`Saved`)
            },
          })
        }
        footer={
          <>
            <ErrorLine error={patch.error} className="min-w-0 flex-1" />
            {!patch.error && (
              <span className="min-w-0 flex-1 text-small text-faint" role="status" data-testid="board-save-note">
                {dirty ? <Trans>You have unsaved changes.</Trans> : <Trans>All changes saved.</Trans>}
              </span>
            )}
            <Button type="submit" size="sm" className="ms-auto" disabled={patch.isPending || !dirty || !ok}>
              <Trans>Save</Trans>
            </Button>
          </>
        }
      >
        <Field label={<Trans>Name</Trans>} htmlFor="board-name">
          <Input
            id="board-name"
            value={name}
            maxLength={100}
            onChange={(e) => setName(e.target.value)}
            data-testid="board-setting-name"
          />
        </Field>
        <Field
          label={<Trans>Description</Trans>}
          htmlFor="board-description"
          hint={<Trans>Shown under the name on the public board.</Trans>}
        >
          <Textarea
            id="board-description"
            value={description}
            maxLength={1000}
            onChange={(e) => setDescription(e.target.value)}
            data-testid="board-setting-description"
          />
        </Field>
        <Field
          label={<Trans>Address</Trans>}
          htmlFor="board-slug"
          hint={<Trans>The board is at {address}. The old address keeps redirecting if you change it.</Trans>}
          error={slug && !validSlug(slug) ? <Trans>Use 3 to 48 letters, numbers and inner hyphens.</Trans> : undefined}
        >
          <Input
            id="board-slug"
            value={slug}
            maxLength={48}
            spellCheck={false}
            autoCapitalize="none"
            onChange={(e) => setSlug(e.target.value.toLowerCase())}
            data-testid="board-setting-slug"
          />
        </Field>
        <div className="flex flex-col gap-2" data-testid="board-suggestions">
          <span className="text-body font-medium" id="board-suggestions-label">
            <Trans>Suggestions from readers</Trans>
          </span>
          <Segmented
            value={suggestions}
            onChange={setSuggestions}
            items={modes.map(([value, label]) => ({ value, label, testId: `board-suggestions-${value}` }))}
            label={t`Suggestions from readers`}
            className="w-fit max-w-full"
          />
          <p className="text-small text-muted-foreground">{modeHint[suggestions]}</p>
        </div>
        <div className="grid gap-5 sm:grid-cols-2">
          <Field
            label={<Trans>Chat channel</Trans>}
            htmlFor="board-channel"
            hint={<Trans>The public board embeds this chat, and ideas your team starts are filed on it.</Trans>}
          >
            <ChoiceSelect
              id="board-channel"
              value={channel}
              onChange={setChannel}
              options={channels.map((c) => [c.id, c.name] as const)}
              label={t`Chat channel`}
            />
          </Field>
          {properties.length > 0 && (
            <Field
              label={<Trans>Analytics property</Trans>}
              htmlFor="board-property"
              hint={
                <>
                  <Trans>Counts page views of the public board without cookies.</Trans>
                  {property && (
                    <>
                      {" "}
                      <Link
                        to={`/analytics/${property}`}
                        className="text-brand underline-offset-4 hover:underline"
                        data-testid="board-property-open"
                      >
                        <Trans>Open Analytics</Trans>
                      </Link>
                    </>
                  )}
                </>
              }
            >
              <ChoiceSelect
                id="board-property"
                value={property}
                onChange={setProperty}
                options={[["", t`None`] as const, ...properties.map((p) => [p.id, p.name] as const)]}
                label={t`Analytics property`}
              />
            </Field>
          )}
        </div>
      </FormCard>
    </Section>
  )
}

function Danger({ board }: { board: RoadmapBoard }) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const [confirm, dialog] = useConfirm()
  const remove = useDeleteBoard(board.id)
  const boardName = board.name
  return (
    <>
      <Section title={<Trans>Delete</Trans>}>
        <Card tone="danger">
          <div className="flex flex-wrap items-center gap-3">
            <p className="min-w-0 flex-1 text-body">
              <Trans>
                Deletes the board with its ideas, votes, replies and stage history. Linked feedback stays, and its
                progress too. The public address stops working.
              </Trans>
            </p>
            <Button
              variant="destructive"
              size="sm"
              disabled={remove.isPending}
              onClick={() =>
                confirm({
                  title: <Trans>Delete {boardName}?</Trans>,
                  description: <Trans>Its ideas, votes, replies and stage history are deleted with it.</Trans>,
                  confirm: <Trans>Delete board</Trans>,
                  run: () =>
                    remove.mutate(undefined, {
                      onSuccess: () => {
                        toast(t`Board deleted`)
                        navigate("/board")
                      },
                    }),
                })
              }
              data-testid="board-delete"
            >
              <Trans>Delete board</Trans>
            </Button>
          </div>
          <ErrorLine error={remove.error} />
        </Card>
      </Section>
      {dialog}
    </>
  )
}

export function Settings({ board }: { board: RoadmapBoard }) {
  const domainSave = useUpdateBoard(board.id)
  return (
    <div className="flex max-w-[640px] flex-col gap-8 px-6 pb-24 phone:px-4" data-testid="board-settings">
      <Visibility board={board} />
      <General board={board} />
      <OwnDomainCard
        domain={board.custom_domain}
        enabled={!!board.own_domains}
        example="roadmap.example.com"
        tid="board-domain"
        save={domainSave}
      />
      <Danger board={board} />
    </div>
  )
}
