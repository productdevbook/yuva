import { Trans, useLingui } from "@lingui/react/macro"
import { useState } from "react"
import { Link } from "react-router"

import { ErrorLine, toast } from "@/components/common"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Textarea } from "@/components/ui/textarea"
import { providerName } from "@/features/issues/providers"
import { useIssueIntegrations, useIssueLinkMutations, type IssueSubject } from "@/features/issues/queries"
import { ChoiceSelect, Field } from "@/features/settings/ui"
import { useSession } from "@/lib/session"

export type IssueDraft = { title: string; body: string }

export default function IssueDialog({
  subject,
  mode,
  draft,
  onClose,
}: {
  subject: IssueSubject
  mode: "create" | "link"
  draft: IssueDraft
  onClose: () => void
}) {
  const { t } = useLingui()
  const { canManage } = useSession()
  const integrations = useIssueIntegrations()
  const { add } = useIssueLinkMutations(subject)
  const [picked, setPicked] = useState("")
  const [title, setTitle] = useState(draft.title)
  const [body, setBody] = useState(draft.body)
  const [target, setTarget] = useState("")
  const [url, setUrl] = useState("")
  const items = integrations.data ?? []
  const integration = items.find((i) => i.id === picked) ?? items[0]
  const create = mode === "create"
  const ready = !!integration && !add.isPending && (create ? title.trim().length > 0 : url.trim().length > 0)
  const submit = () => {
    if (!integration || !ready) return
    const base = { integration_id: integration.id }
    add.mutate(
      create
        ? { ...base, title: title.trim(), body: body.trim() || undefined, target: target.trim() || undefined }
        : { ...base, url: url.trim() },
      {
        onSuccess: (l) => {
          const key = l.key
          toast(create ? t`Created ${key}` : t`Linked ${key}`)
          onClose()
        },
      }
    )
  }
  const forIdea = subject.kind === "idea"
  const targetHint = integration?.default_target ? (
    <Trans>Leave empty for {integration.default_target}.</Trans>
  ) : integration?.provider === "linear" ? (
    <Trans>A team key such as ENG.</Trans>
  ) : (
    <Trans>As owner/repo.</Trans>
  )
  return (
    <Dialog open onOpenChange={(o) => !o && !add.isPending && onClose()}>
      <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-xl" data-testid={`issue-dialog-${mode}`}>
        <form
          className="flex flex-col gap-5"
          onSubmit={(e) => {
            e.preventDefault()
            submit()
          }}
        >
          <DialogHeader>
            <DialogTitle>{create ? <Trans>Create an issue</Trans> : <Trans>Link an issue</Trans>}</DialogTitle>
            <DialogDescription>
              {create ? (
                forIdea ? (
                  <Trans>
                    Creates an issue in your tracker and links it to this idea. Only the text below is sent, with a link
                    back to the idea.
                  </Trans>
                ) : (
                  <Trans>
                    Creates an issue in your tracker and links it to this feedback. Only the text below is sent, with a
                    link back to the conversation; the customer's name and address stay here.
                  </Trans>
                )
              ) : forIdea ? (
                <Trans>Paste the address of an existing issue to show it on this idea.</Trans>
              ) : (
                <Trans>
                  Paste the address of an existing issue. Its state then sets the progress the customer sees.
                </Trans>
              )}
            </DialogDescription>
          </DialogHeader>
          {integrations.isPending ? (
            <Skeleton className="h-20 w-full" />
          ) : items.length === 0 ? (
            <p className="text-body text-muted-foreground" data-testid="issue-no-tracker">
              {canManage ? (
                <Trans>
                  No tracker is connected yet. Connect GitHub or Linear in{" "}
                  <Link to="/settings/issue-trackers" className="underline" onClick={onClose}>
                    the settings
                  </Link>
                  .
                </Trans>
              ) : (
                <Trans>No tracker is connected yet. Ask an owner or admin to connect GitHub or Linear.</Trans>
              )}
            </p>
          ) : (
            <>
              {items.length > 1 && (
                <Field label={<Trans>Tracker</Trans>} htmlFor="issue-tracker">
                  <ChoiceSelect
                    id="issue-tracker"
                    value={integration?.id ?? ""}
                    onChange={setPicked}
                    options={items.map((i) => [i.id, `${i.name} · ${providerName(i.provider)}`] as const)}
                    label={t`Tracker`}
                  />
                </Field>
              )}
              {create ? (
                <>
                  <Field label={<Trans>Title</Trans>} htmlFor="issue-title">
                    <Input
                      id="issue-title"
                      required
                      autoFocus
                      maxLength={256}
                      value={title}
                      onChange={(e) => setTitle(e.target.value)}
                      data-testid="issue-title"
                    />
                  </Field>
                  <Field label={<Trans>Description</Trans>} htmlFor="issue-body" hint={<Trans>Markdown works.</Trans>}>
                    <Textarea
                      id="issue-body"
                      rows={6}
                      maxLength={20000}
                      value={body}
                      onChange={(e) => setBody(e.target.value)}
                      className="max-h-64"
                      data-testid="issue-body"
                    />
                  </Field>
                  <Field label={<Trans>Where</Trans>} htmlFor="issue-target" hint={targetHint}>
                    <Input
                      id="issue-target"
                      value={target}
                      onChange={(e) => setTarget(e.target.value)}
                      placeholder={
                        integration?.default_target ?? (integration?.provider === "linear" ? "ENG" : "owner/repo")
                      }
                      autoCapitalize="off"
                      spellCheck={false}
                      data-testid="issue-target"
                    />
                  </Field>
                </>
              ) : (
                <Field
                  label={<Trans>Issue address</Trans>}
                  htmlFor="issue-url"
                  hint={<Trans>An issue URL, owner/repo#12 or ENG-123.</Trans>}
                >
                  <Input
                    id="issue-url"
                    required
                    autoFocus
                    value={url}
                    onChange={(e) => setUrl(e.target.value)}
                    autoCapitalize="off"
                    spellCheck={false}
                    data-testid="issue-url"
                  />
                </Field>
              )}
            </>
          )}
          <ErrorLine error={add.error ?? integrations.error} />
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={onClose}>
              <Trans>Cancel</Trans>
            </Button>
            <Button type="submit" disabled={!ready} data-testid="issue-submit">
              {create ? <Trans>Create issue</Trans> : <Trans>Link issue</Trans>}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
