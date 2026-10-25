import { Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, LoaderIcon, SendIcon, UserRoundIcon, XIcon } from "lucide-react"
import { useState } from "react"

import { CheckItem, CheckList, ErrorLine, MemberAvatar } from "@/components/common"
import { BROADCAST_KEYS, keyLabel } from "@/components/common/ShortcutSheet"
import { formatRelative, useLanguageName } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { useSendTest } from "@/features/broadcasts/broadcastQueries"
import { ContactSearch } from "@/features/contact/ContactSearch"
import { fallbackOf, versions } from "@/features/designs/doc"
import { useDesign } from "@/features/designs/queries"
import { contactName } from "@/features/contact/Section"
import type { Broadcast, Contact } from "@/lib/api"
import { useSession } from "@/lib/session"
import { useMembers } from "@/lib/workspace"

const MAX = 5
const AUTO = "auto"

export function useTestSend(id: string) {
  const { membership } = useSession()
  const [members, setMembers] = useState<string[]>([membership.member_id])
  const [contact, setContact] = useState<Contact | null>(null)
  const [locale, setLocale] = useState(AUTO)
  const send = useSendTest(id)
  const run = () => {
    if (members.length === 0 || send.isPending) return
    send.mutate({ member_ids: members, contact_id: contact?.id, locale: locale === AUTO ? undefined : locale })
  }
  return { members, setMembers, contact, setContact, locale, setLocale, send, run }
}

export function TestSend({
  b,
  state,
  disabled,
}: {
  b: Broadcast
  state: ReturnType<typeof useTestSend>
  disabled: boolean
}) {
  const { t, i18n } = useLingui()
  const team = useMembers().data ?? []
  const [picking, setPicking] = useState(false)
  const { members, setMembers, contact, setContact, locale, setLocale, send, run } = state
  const languageName = useLanguageName()
  const doc = useDesign(b.design_id ?? "").data?.document
  const tags = doc ? versions(doc) : []
  const fallback = languageName(doc ? (fallbackOf(doc) ?? "") : "")
  const languages: Record<string, string> = {
    [AUTO]: contact ? t`The contact’s language` : t`${fallback} (fallback)`,
    ...Object.fromEntries(tags.map((tag) => [tag, languageName(tag)])),
  }
  const chosen = new Set(members)
  const unnamed = t`Unnamed contact`
  const who = contact ? contactName(contact, unnamed) : ""
  const key = keyLabel(BROADCAST_KEYS.test).join("")
  const last = b.last_test_at ? formatRelative(b.last_test_at, i18n.locale) : null
  const sentTo = send.data?.sent_to.join(", ")
  return (
    <div className="flex flex-col gap-4" data-testid="broadcast-test">
      <CheckList
        labelId="test-members"
        label={<Trans>Send to</Trans>}
        hint={
          <p className="text-small text-faint">
            <Trans>
              Tests go only to members of the workspace, at most five at a time, with [Test] before the subject.
            </Trans>
          </p>
        }
      >
        {team.map((m) => (
          <CheckItem
            key={m.id}
            checked={chosen.has(m.id)}
            disabled={disabled || (!chosen.has(m.id) && members.length >= MAX)}
            onChange={(on) => setMembers(on ? [...members, m.id] : members.filter((id) => id !== m.id))}
            testId="test-member"
          >
            <span className="flex min-w-0 items-center gap-2">
              <MemberAvatar name={m.name || m.email} className="size-5" />
              <span className="min-w-0 truncate">{m.name || m.email}</span>
            </span>
            {m.name && (
              <span className="truncate text-small text-faint" dir="ltr">
                {m.email}
              </span>
            )}
          </CheckItem>
        ))}
      </CheckList>
      <div className="flex flex-col gap-2">
        <span className="text-body font-medium">
          <Trans>Merge fields</Trans>
        </span>
        {contact ? (
          <div className="flex items-center gap-2 rounded-xl border px-3.5 py-2 text-body" data-testid="test-contact">
            <UserRoundIcon className="size-4 shrink-0 text-faint" />
            <span className="min-w-0 flex-1 truncate">
              <Trans>Filled with the values of {who}</Trans>
            </span>
            {!disabled && (
              <Button variant="ghost" size="icon-sm" onClick={() => setContact(null)} aria-label={t`Use sample values`}>
                <XIcon />
              </Button>
            )}
          </div>
        ) : (
          <div className="flex flex-wrap items-center gap-2 text-body text-muted-foreground">
            <span className="min-w-0 flex-1">
              <Trans>Filled with sample values.</Trans>
            </span>
            {!disabled && (
              <Button variant="outline" size="sm" onClick={() => setPicking(true)} data-testid="test-pick-contact">
                <Trans>Use a contact’s values</Trans>
              </Button>
            )}
          </div>
        )}
      </div>
      {tags.length > 1 && (
        <div className="flex flex-col gap-2">
          <span id="test-language" className="text-body font-medium">
            <Trans>Language</Trans>
          </span>
          <Select
            value={tags.includes(locale) ? locale : AUTO}
            items={languages}
            disabled={disabled}
            onValueChange={(v) => setLocale(String(v ?? AUTO))}
          >
            <SelectTrigger className="w-full min-w-0" aria-labelledby="test-language" data-testid="test-language">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {Object.entries(languages).map(([value, label]) => (
                <SelectItem key={value} value={value}>
                  {label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      )}
      <div className="flex flex-wrap items-center gap-3">
        <Button
          variant="outline"
          onClick={run}
          disabled={disabled || members.length === 0 || send.isPending}
          title={t`Send a test (${key})`}
          data-testid="send-test"
        >
          {send.isPending ? <LoaderIcon className="animate-spin" /> : <SendIcon />}
          <Trans>Send a test</Trans>
        </Button>
        {send.isSuccess ? (
          <span className="inline-flex min-w-0 items-center gap-1.5 text-body text-muted-foreground" role="status">
            <CheckIcon className="size-4 shrink-0 text-success" />
            <span className="min-w-0 truncate">
              <Trans>On its way to {sentTo}</Trans>
            </span>
          </span>
        ) : (
          last && (
            <span className="text-small text-faint" data-testid="last-test">
              <Trans>Last test {last}</Trans>
            </span>
          )
        )}
      </div>
      <ErrorLine error={send.error} />
      <Dialog open={picking} onOpenChange={setPicking}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              <Trans>Fill the test with a contact’s values</Trans>
            </DialogTitle>
            <DialogDescription>
              <Trans>The test still goes only to the members you picked, never to the contact.</Trans>
            </DialogDescription>
          </DialogHeader>
          <ContactSearch
            exclude=""
            onPick={(c) => {
              setContact(c)
              setPicking(false)
            }}
          />
        </DialogContent>
      </Dialog>
    </div>
  )
}

export function TestSendDialog({
  b,
  state,
  disabled,
  onClose,
}: {
  b: Broadcast
  state: ReturnType<typeof useTestSend>
  disabled: boolean
  onClose: () => void
}) {
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent data-testid="test-dialog">
        <DialogHeader>
          <DialogTitle>
            <Trans>Send a test</Trans>
          </DialogTitle>
        </DialogHeader>
        <TestSend b={b} state={state} disabled={disabled} />
      </DialogContent>
    </Dialog>
  )
}
