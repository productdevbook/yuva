import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { ArrowRightIcon, CheckIcon, InfoIcon, PlusIcon, XIcon } from "lucide-react"
import { useState } from "react"
import { useNavigate } from "react-router"

import { CheckItem, CheckList, ErrorLine, Notice, Segmented, StepHeader } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { emailForm, emailInput } from "@/features/settings/channels/email/form"
import { slugify } from "@/features/settings/inboxes/slugify"
import { useNumber } from "@/features/broadcasts/BroadcastParts"
import { ChoiceSelect, Field } from "@/features/settings/ui"
import { useMigrationFolders, useMigrationStep, useUpdateMigration } from "@/features/migrations/queries"
import { isHelpdesk } from "@/features/migrations/text"
import { WizardLayout } from "@/features/migrations/Wizard"
import { browserLanguage, browserTimeZone } from "@/features/setup/setup"
import {
  api,
  ApiError,
  unwrap,
  type Migration,
  type MigrationInboxMapping,
  type MigrationLabelMapping,
  type MigrationPerson,
  type MigrationPreset,
} from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { useInboxes, useLabels, useMembers } from "@/lib/workspace"

const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
const NEW_INBOX = "__new"
const KEEP_NAME = "__name"
const SAME_INBOX = "__same"

function sameAddress(a: string, b: string) {
  return a.trim().toLowerCase() === b.trim().toLowerCase()
}

function labelValue(l: Pick<MigrationLabelMapping, "action" | "label_id">) {
  return l.action === "existing" && l.label_id ? `existing:${l.label_id}` : l.action
}

export function MapStep({ m, preset }: { m: Migration; preset: MigrationPreset }) {
  const { t } = useLingui()
  const num = useNumber()
  const qc = useQueryClient()
  const navigate = useNavigate()
  const { workspaceId: ws } = useSession()
  const inboxes = useInboxes().data ?? []
  const members = useMembers().data ?? []
  const labels = useLabels().data ?? []
  const folders = useMigrationFolders(m.id).data ?? []
  const update = useUpdateMigration(m.id)
  const scan = useMigrationStep(m.id, "scan")
  const canEdit = m.actions.includes("update")
  const address = m.source_address
  const desk = isHelpdesk(m.source)
  const provider = preset.name

  const [inboxId, setInboxId] = useState(m.inbox_id ?? "")
  const [newName, setNewName] = useState("")
  const [team, setTeam] = useState<string[]>(m.options.team_addresses)
  const [adding, setAdding] = useState("")
  const [people, setPeople] = useState<Record<string, string>>(() =>
    Object.fromEntries(m.options.people.map((p) => [p.address.toLowerCase(), p.member_id ?? KEEP_NAME]))
  )
  const [labelMap, setLabelMap] = useState<Record<string, string>>(() =>
    Object.fromEntries(m.options.labels.map((l) => [l.folder, labelValue(l)]))
  )
  const [closeAll, setCloseAll] = useState(m.options.close_after_days === 0)
  const [days, setDays] = useState(String(m.options.close_after_days || 14))
  const [keep, setKeep] = useState(m.options.keep_originals)
  const [inboxMap, setInboxMap] = useState<Record<string, string>>(() =>
    Object.fromEntries((m.options.inboxes ?? []).map((i) => [i.folder, i.inbox_id]))
  )

  const channels = useQuery({
    queryKey: keys.channels(ws, inboxId),
    queryFn: () =>
      unwrap(api.GET("/v1/inboxes/{inboxId}/channels", { params: { path: { inboxId } } })).then((r) => r.items),
    enabled: !!inboxId && inboxId !== NEW_INBOX,
  })
  const channel = channels.data?.find((c) => c.kind === "email" && c.email && sameAddress(c.email.address, address))
  const inbox = inboxes.find((i) => i.id === inboxId)
  const inboxName = inbox?.name ?? ""
  const channelName = channel?.name ?? ""

  const createInbox = useMutation({
    mutationFn: async () => {
      const name = newName.trim()
      const base = slugify(name) || "inbox"
      const body = (slug: string) => ({
        name,
        slug,
        mode: "async" as const,
        ask_for_rating: false,
        default_locale: browserLanguage(),
        timezone: browserTimeZone(),
      })
      try {
        return await unwrap(api.POST("/v1/inboxes", { body: body(base) }))
      } catch (err) {
        if (!(err instanceof ApiError) || err.status !== 409) throw err
        return unwrap(api.POST("/v1/inboxes", { body: body(`${base}-${Math.random().toString(36).slice(2, 6)}`) }))
      }
    },
    onSuccess: (r) => {
      void qc.invalidateQueries({ queryKey: keys.inboxes(ws) })
      setInboxId(r.inbox.id)
      setNewName("")
    },
  })

  const createChannel = useMutation({
    mutationFn: () =>
      unwrap(
        api.POST("/v1/inboxes/{inboxId}/channels", {
          params: { path: { inboxId } },
          body: {
            kind: "email",
            name: inbox?.name || t`E-mail`,
            email: emailInput({ ...emailForm(), address }),
          },
        })
      ),
    onSuccess: (ch) => {
      qc.setQueryData(keys.channel(ws, ch.id), ch)
      void qc.invalidateQueries({ queryKey: keys.channels(ws, inboxId) })
    },
  })

  const daysShown = Number(days) || 14
  const senders = [address, ...team]
  const memberFor = (a: string) => {
    const set = people[a.toLowerCase()]
    if (set) return set
    return members.find((x) => sameAddress(x.email, a))?.id ?? KEEP_NAME
  }
  const labelFolders = desk
    ? [
        ...new Set([
          ...m.options.labels.map((l) => l.folder),
          ...(m.estimate?.labels ?? []).filter((l) => l.action === "create").map((l) => l.name),
        ]),
      ]
        .sort((a, b) => a.localeCompare(b))
        .map((name) => ({ id: name, name }))
    : folders.filter((f) => (f.kind === "user" || f.kind === "label") && f.use !== "skip")
  const deskFolders = desk ? folders.filter((f) => f.selected) : []
  const inboxFor = (folder: string) => {
    const f = folders.find((x) => x.name === folder)
    const set = inboxMap[folder] ?? (f?.inbox_id !== m.inbox_id ? f?.inbox_id : undefined)
    return set && set !== inboxId && inboxes.some((i) => i.id === set) ? set : SAME_INBOX
  }
  const agents = desk ? (m.team_suggestions ?? []) : []
  const labelFor = (folder: string) => {
    const set = labelMap[folder]
    if (set) return set
    const f = desk ? undefined : folders.find((x) => x.name === folder)
    return f ? labelValue({ action: f.label_action, label_id: f.label_id }) : "create"
  }
  const labelOptions = (folder: string) =>
    [
      ["create", t`New label “${folder}”`] as const,
      ["none", t`No label`] as const,
      ...labels.map((l) => [`existing:${l.id}`, l.name] as const),
    ] as const

  const addTeam = () => {
    const a = adding.trim()
    if (!EMAIL.test(a) || sameAddress(a, address) || team.some((x) => sameAddress(x, a))) return
    setTeam([...team, a])
    setAdding("")
  }

  const suggestions = m.team_suggestions ?? []
  const isTeam = (a: string) => team.some((x) => sameAddress(x, a))
  const toggleTeam = (a: string, on: boolean) =>
    setTeam(on ? (isTeam(a) ? team : [...team, a]) : team.filter((x) => !sameAddress(x, a)))

  const save = useMutation({
    mutationFn: async () => {
      const peopleBody: MigrationPerson[] = (desk ? agents.map((a) => a.address) : senders).map((a) => {
        const id = memberFor(a)
        return id === KEEP_NAME ? { address: a } : { address: a, member_id: id }
      })
      const labelsBody: MigrationLabelMapping[] = labelFolders.map((f) => {
        const v = labelFor(f.name)
        return v.startsWith("existing:")
          ? { folder: f.name, action: "existing", label_id: v.slice("existing:".length) }
          : { folder: f.name, action: v as "create" | "none" }
      })
      const inboxesBody: MigrationInboxMapping[] = deskFolders.flatMap((f) => {
        const id = inboxFor(f.name)
        return id === SAME_INBOX ? [] : [{ folder: f.name, inbox_id: id }]
      })
      const next = await update.mutateAsync({
        inbox_id: inboxId,
        channel_id: channel!.id,
        options: {
          team_addresses: team,
          people: peopleBody,
          labels: labelsBody,
          close_after_days: desk
            ? m.options.close_after_days
            : closeAll
              ? 0
              : Math.max(1, Math.min(3650, Number(days) || 14)),
          keep_originals: keep,
          ...(desk ? { inboxes: inboxesBody } : {}),
        },
      })
      if (next.state === "connected" && next.actions.includes("scan")) await scan.mutateAsync()
      return next
    },
    onSuccess: () => navigate(`/migrations/${m.id}/review`),
  })

  const taken = createChannel.error instanceof ApiError && createChannel.error.code === "email_address_taken"
  const ready = !!inboxId && inboxId !== NEW_INBOX && !!channel
  return (
    <WizardLayout
      step="map"
      m={{ ...m, inbox_id: inboxId && inboxId !== NEW_INBOX ? inboxId : m.inbox_id }}
      preset={preset}
    >
      <StepHeader title={<Trans>Where does it go?</Trans>}>
        {desk ? (
          <Trans>
            Pick the inboxes for {provider}'s conversations, which member each agent is, and how tags become labels.
            Changing these after the scan plans the import again in a moment.
          </Trans>
        ) : (
          <Trans>
            Pick the inbox for {address}, who on your team wrote what, and how folders become labels. Changing these
            after the scan plans the import again in a moment.
          </Trans>
        )}
      </StepHeader>

      {!canEdit && (
        <Notice icon={InfoIcon} className="mb-6">
          <p>
            <Trans>The scan is running. You can change these when it is done.</Trans>
          </p>
        </Notice>
      )}

      <form
        className="flex flex-col gap-8"
        onSubmit={(e) => {
          e.preventDefault()
          if (ready && canEdit) save.mutate()
        }}
      >
        <fieldset className="flex min-w-0 flex-col gap-4" disabled={!canEdit}>
          <legend className="mb-3 text-title">
            <Trans>Inbox</Trans>
          </legend>
          <Field label={<Trans>Inbox</Trans>} htmlFor="migration-inbox">
            <ChoiceSelect
              id="migration-inbox"
              label={t`Inbox`}
              value={inboxId || ""}
              onChange={setInboxId}
              placeholder={t`Pick an inbox`}
              options={[...inboxes.map((i) => [i.id, i.name] as const), [NEW_INBOX, t`New inbox…`] as const]}
              className="h-10 w-full rounded-xl"
            />
          </Field>
          {inboxId === NEW_INBOX && (
            <div className="flex items-end gap-2">
              <Field label={<Trans>Name of the new inbox</Trans>} htmlFor="migration-inbox-name" className="flex-1">
                <Input
                  id="migration-inbox-name"
                  value={newName}
                  maxLength={200}
                  onChange={(e) => setNewName(e.target.value)}
                  placeholder={t`For example Fieldnote`}
                  className="h-10 rounded-xl"
                  data-testid="migration-inbox-name"
                />
              </Field>
              <Button
                type="button"
                variant="outline"
                className="h-10"
                onClick={() => createInbox.mutate()}
                disabled={!newName.trim() || createInbox.isPending}
              >
                <Trans>Create</Trans>
              </Button>
            </div>
          )}
          <ErrorLine error={createInbox.error} />
          {inbox && channels.data && (
            <div data-testid="migration-channel">
              {channel ? (
                <p className="flex items-center gap-2 text-body text-muted-foreground">
                  <CheckIcon className="size-4 shrink-0 text-success" />
                  <Trans>
                    Mail goes into the e-mail channel “{channelName}”, which receives {address}.
                  </Trans>
                </p>
              ) : (
                <Notice icon={InfoIcon}>
                  <p>
                    <Trans>
                      {inboxName} has no e-mail channel for {address} yet. History is stored in that channel, so replies
                      after the switch go out from the same address.
                    </Trans>
                  </p>
                  {taken ? (
                    <p className="text-destructive" role="alert">
                      <Trans>
                        Another channel already receives mail at {address}, in this workspace or another. Use that
                        channel's inbox, or remove the address there first.
                      </Trans>
                    </p>
                  ) : (
                    <ErrorLine error={createChannel.error} />
                  )}
                  <Button
                    type="button"
                    size="sm"
                    className="self-start"
                    onClick={() => createChannel.mutate()}
                    disabled={createChannel.isPending}
                    data-testid="migration-create-channel"
                  >
                    <PlusIcon />
                    <Trans>Create the e-mail channel</Trans>
                  </Button>
                </Notice>
              )}
            </div>
          )}
        </fieldset>

        {desk && deskFolders.length > 0 && (
          <fieldset className="flex min-w-0 flex-col gap-4" disabled={!canEdit} data-testid="migration-desk-inboxes">
            <legend className="mb-1 text-title">
              {m.source === "zendesk" ? (
                <Trans>Brands</Trans>
              ) : m.source === "front" ? (
                <Trans>Inboxes</Trans>
              ) : (
                <Trans>Mailboxes</Trans>
              )}
            </legend>
            <p className="text-small text-faint">
              <Trans>
                Each goes into its inbox's e-mail channel for its address, else that inbox's oldest e-mail channel, so
                replies reach the customer. An inbox without an e-mail channel keeps them as history only.
              </Trans>
            </p>
            <ul className="flex flex-col divide-y rounded-xl border">
              {deskFolders.map(({ id, name, address: folderAddress }) => (
                <li key={id} className="flex flex-wrap items-center gap-x-3 gap-y-2 px-3.5 py-2.5">
                  <span className="flex min-w-0 flex-1 basis-40 flex-col">
                    <span className="truncate text-body">{name}</span>
                    {folderAddress && <span className="truncate text-small text-faint">{folderAddress}</span>}
                  </span>
                  <ChoiceSelect
                    label={t`Inbox for ${name}`}
                    value={inboxFor(name)}
                    onChange={(v) => setInboxMap({ ...inboxMap, [name]: v })}
                    options={[
                      [SAME_INBOX, inbox ? t`Same as above: ${inboxName}` : t`Same as above`] as const,
                      ...inboxes.filter((i) => i.id !== inboxId).map((i) => [i.id, i.name] as const),
                    ]}
                    className="h-9 w-48 rounded-lg"
                  />
                </li>
              ))}
            </ul>
          </fieldset>
        )}

        {desk && (
          <fieldset className="flex min-w-0 flex-col gap-4" disabled={!canEdit} data-testid="migration-agents">
            <legend className="mb-1 text-title">
              <Trans>Agents</Trans>
            </legend>
            <p className="text-small text-faint">
              <Trans>
                An agent's replies, notes and assigned conversations become the member's. Without a member, the agent's
                name is kept.
              </Trans>
            </p>
            {agents.length === 0 ? (
              <p className="text-small text-faint">
                <Trans>The scan found no agents.</Trans>
              </p>
            ) : (
              <ul className="flex flex-col divide-y rounded-xl border" aria-label={t`Agents`}>
                {agents.map(({ address: sender, name: agent, messages: assigned }) => (
                  <li key={sender} className="flex flex-wrap items-center gap-x-3 gap-y-2 px-3.5 py-2.5">
                    <span className="flex min-w-0 flex-1 basis-40 flex-col">
                      <span className="truncate text-body">{agent || sender}</span>
                      <span className="truncate text-small text-faint">
                        {agent && <>{sender} · </>}
                        <Plural value={assigned} one="# assigned conversation" other="# assigned conversations" />
                      </span>
                    </span>
                    <ChoiceSelect
                      label={t`Team member for ${sender}`}
                      value={memberFor(sender)}
                      onChange={(v) => setPeople({ ...people, [sender.toLowerCase()]: v })}
                      options={[
                        ...members.map((x) => [x.id, x.name || x.email] as const),
                        [KEEP_NAME, t`Keep the name only`] as const,
                      ]}
                      className="h-9 w-48 rounded-lg"
                    />
                  </li>
                ))}
              </ul>
            )}
          </fieldset>
        )}

        {!desk && (
          <fieldset className="flex min-w-0 flex-col gap-4" disabled={!canEdit}>
            <legend className="mb-1 text-title">
              <Trans>Your team</Trans>
            </legend>
            <p className="text-small text-faint">
              <Trans>
                Mail from these addresses is imported as sent by your team. Your members' sign-in addresses count too.
              </Trans>
            </p>
            {suggestions.length > 0 && (
              <CheckList
                labelId="migration-team-suggestions"
                label={<Trans>Found in your Sent folders</Trans>}
                hint={
                  <p className="text-small text-faint">
                    <Trans>
                      Tick the addresses your team writes from, so their mail is not imported as mail from customers.
                    </Trans>
                  </p>
                }
              >
                {suggestions.map((s) => {
                  const count = num(s.messages)
                  return (
                    <CheckItem
                      key={s.address}
                      checked={isTeam(s.address)}
                      disabled={!canEdit}
                      onChange={(on) => toggleTeam(s.address, on)}
                      testId="migration-team-suggestion"
                    >
                      <span className="truncate">
                        {s.name ? (
                          <>
                            {s.name} <span className="text-faint">{s.address}</span>
                          </>
                        ) : (
                          s.address
                        )}
                      </span>
                      <span className="text-small text-faint">
                        {s.messages === 1 ? (
                          <Trans>Sent 1 message from this address</Trans>
                        ) : (
                          <Trans>Sent {count} messages from this address</Trans>
                        )}
                      </span>
                    </CheckItem>
                  )
                })}
              </CheckList>
            )}
            <ul className="flex flex-col divide-y rounded-xl border" aria-label={t`Team addresses`}>
              {senders.map((sender) => (
                <li key={sender} className="flex flex-wrap items-center gap-x-3 gap-y-2 px-3.5 py-2.5">
                  <span className="min-w-0 flex-1 basis-40 truncate text-body">{sender}</span>
                  <ChoiceSelect
                    label={t`Team member for ${sender}`}
                    value={memberFor(sender)}
                    onChange={(v) => setPeople({ ...people, [sender.toLowerCase()]: v })}
                    options={[
                      ...members.map((x) => [x.id, x.name || x.email] as const),
                      [KEEP_NAME, t`Keep the name only`] as const,
                    ]}
                    className="h-9 w-48 rounded-lg"
                  />
                  {sender !== address && (
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon-sm"
                      onClick={() => toggleTeam(sender, false)}
                      aria-label={t`Remove ${sender}`}
                    >
                      <XIcon />
                    </Button>
                  )}
                </li>
              ))}
            </ul>
            <div className="flex items-end gap-2">
              <Field label={<Trans>Another team address</Trans>} htmlFor="migration-team-add" className="flex-1">
                <Input
                  id="migration-team-add"
                  type="email"
                  value={adding}
                  onChange={(e) => setAdding(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") {
                      e.preventDefault()
                      addTeam()
                    }
                  }}
                  placeholder={t`sales@example.com`}
                  className="h-10 rounded-xl"
                  data-testid="migration-team-add"
                />
              </Field>
              <Button
                type="button"
                variant="outline"
                className="h-10"
                onClick={addTeam}
                disabled={!EMAIL.test(adding.trim())}
              >
                <Trans>Add</Trans>
              </Button>
            </div>
          </fieldset>
        )}

        {labelFolders.length > 0 && (
          <fieldset className="flex min-w-0 flex-col gap-4" disabled={!canEdit}>
            <legend className="mb-1 text-title">{desk ? <Trans>Tags</Trans> : <Trans>Labels</Trans>}</legend>
            <p className="text-small text-faint">
              {desk ? (
                <Trans>{provider}'s tags become Yuva labels on their conversations.</Trans>
              ) : (
                <Trans>Your folders and labels become Yuva labels on the conversations they hold.</Trans>
              )}
            </p>
            <ul className="flex flex-col divide-y rounded-xl border">
              {labelFolders.map(({ id, name }) => (
                <li key={id} className="flex flex-wrap items-center gap-x-3 gap-y-2 px-3.5 py-2.5">
                  <span className="min-w-0 flex-1 basis-40 truncate text-body">{name}</span>
                  <ChoiceSelect
                    label={t`Label for ${name}`}
                    value={labelFor(name)}
                    onChange={(v) => setLabelMap({ ...labelMap, [name]: v })}
                    options={labelOptions(name)}
                    className="h-9 w-auto min-w-48 rounded-lg"
                  />
                </li>
              ))}
            </ul>
          </fieldset>
        )}

        {desk ? (
          <section className="flex flex-col gap-2" aria-labelledby="migration-status">
            <h2 id="migration-status" className="text-title">
              <Trans>Status</Trans>
            </h2>
            <p className="text-small text-faint">
              <Trans>Each conversation keeps the status {provider} reports for it: open, pending or closed.</Trans>
            </p>
          </section>
        ) : (
          <fieldset className="flex min-w-0 flex-col gap-3" disabled={!canEdit}>
            <legend className="mb-3 text-title">
              <Trans>Status</Trans>
            </legend>
            <Segmented
              value={closeAll ? "all" : "older"}
              onChange={(v) => setCloseAll(v === "all")}
              items={[
                { value: "older", label: t`Close older conversations` },
                { value: "all", label: t`Close everything` },
              ]}
              label={t`Status`}
              className="w-full"
              itemClassName="flex-1"
            />
            {!closeAll && (
              <Field
                label={<Trans>Close conversations older than {daysShown} days</Trans>}
                htmlFor="migration-close-days"
              >
                <Input
                  id="migration-close-days"
                  inputMode="numeric"
                  value={days}
                  onChange={(e) => setDays(e.target.value.replace(/\D/g, ""))}
                  className="h-9 w-24 rounded-lg"
                  data-testid="migration-close-days"
                />
              </Field>
            )}
            <p className="text-small text-faint">
              {closeAll ? (
                <Trans>Every imported conversation is closed.</Trans>
              ) : (
                <Trans>Newer ones stay open when the customer wrote last, pending when your team did.</Trans>
              )}
            </p>
          </fieldset>
        )}

        {m.source !== "zendesk" && m.source !== "front" && (
          <Label className="items-start justify-between gap-4 font-normal">
            <span className="flex flex-col gap-0.5">
              <span className="font-medium">
                <Trans>Keep original messages</Trans>
              </span>
              <span className="text-small text-faint">
                <Trans>
                  Each message's original source is stored, as for mail that arrives live. Uses more storage.
                </Trans>
              </span>
            </span>
            <Switch checked={keep} onCheckedChange={setKeep} disabled={!canEdit} />
          </Label>
        )}

        <ErrorLine error={save.error} />
        <Button
          type="submit"
          size="lg"
          className="self-start"
          disabled={!ready || !canEdit || save.isPending}
          data-testid="migration-map-continue"
        >
          <Trans>Continue</Trans>
          <ArrowRightIcon className="rtl:rotate-180" />
        </Button>
      </form>
    </WizardLayout>
  )
}
