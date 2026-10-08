import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { useNavigate } from "react-router"

import { ErrorLine, SecretDialog } from "@/components/common"
import { MODES, useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { slugify } from "@/features/settings/inboxes/slugify"
import { Field } from "@/features/settings/ui"
import { api, unwrap, type InboxMode } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"

export function CreateInboxDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (o: boolean) => void }) {
  const { t } = useLingui()
  const qc = useQueryClient()
  const navigate = useNavigate()
  const { workspaceId: ws } = useSession()
  const text = useEnumText()
  const [name, setName] = useState("")
  const [slug, setSlug] = useState("")
  const [slugTouched, setSlugTouched] = useState(false)
  const [mode, setMode] = useState<InboxMode>("async")
  const [created, setCreated] = useState<{ id: string; secret: string } | null>(null)
  const create = useMutation({
    mutationFn: () =>
      unwrap(api.POST("/v1/inboxes", { body: { name, slug, mode, timezone: Intl.DateTimeFormat().resolvedOptions().timeZone } })),
    onSuccess: (r) => {
      void qc.invalidateQueries({ queryKey: keys.inboxes(ws) })
      onOpenChange(false)
      setName("")
      setSlug("")
      setSlugTouched(false)
      setCreated({ id: r.inbox.id, secret: r.identity_secret })
    },
  })
  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent>
          <form
            className="flex flex-col gap-5"
            onSubmit={(e) => {
              e.preventDefault()
              create.mutate()
            }}
          >
            <DialogHeader>
              <DialogTitle>
                <Trans>New inbox</Trans>
              </DialogTitle>
              <DialogDescription>
                <Trans>You can change hours, branding and channels after creating it.</Trans>
              </DialogDescription>
            </DialogHeader>
            <Field label={<Trans>Name</Trans>} htmlFor="inbox-name">
              <Input
                id="inbox-name"
                required
                maxLength={200}
                value={name}
                placeholder={t`Acme App`}
                onChange={(e) => {
                  setName(e.target.value)
                  if (!slugTouched) setSlug(slugify(e.target.value))
                }}
              />
            </Field>
            <Field label={<Trans>Slug</Trans>} htmlFor="inbox-slug" hint={<Trans>Lowercase letters, digits and dashes. Used in addresses and the widget.</Trans>}>
              <Input
                id="inbox-slug"
                required
                pattern="[a-z0-9]+(-[a-z0-9]+)*"
                maxLength={64}
                value={slug}
                onChange={(e) => {
                  setSlugTouched(true)
                  setSlug(e.target.value)
                }}
              />
            </Field>
            <Field label={<Trans>Mode</Trans>}>
              <Select value={mode} onValueChange={(v) => setMode(v as InboxMode)} items={text.mode}>
                <SelectTrigger className="w-full" aria-label={t`Mode`}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {MODES.map((m) => (
                    <SelectItem key={m} value={m}>
                      {text.mode[m]}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <ErrorLine error={create.error} />
            <DialogFooter>
              <Button type="submit" disabled={create.isPending}>
                <Trans>Create inbox</Trans>
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <SecretDialog
        secret={created?.secret ?? null}
        title={<Trans>Identity secret</Trans>}
        description={<Trans>Your backend signs identity tokens with this secret. Copy it now and store it safely; it is not shown again.</Trans>}
        onClose={() => {
          const id = created?.id
          setCreated(null)
          if (id) navigate(id)
        }}
      />
    </>
  )
}
