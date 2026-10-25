import { Plural, Trans } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"

import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { look } from "@/features/inbox/PageFrame"
import { Pill } from "@/features/settings/ui"
import { RadioRow, Sec, SecBody, SecFoot, secCard } from "@/features/settings/workspace/parts"
import { api, unwrap } from "@/lib/api"
import { meKey, useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

const PRESETS = [90, 365, 730, 2555]

function Days({ d }: { d: number }) {
  return <Plural value={d} one="# day" other="# days" />
}

export function RetentionForm() {
  const qc = useQueryClient()
  const { membership } = useSession()
  const current = membership.workspace.retention_days?.toString() ?? ""
  const [mode, setMode] = useState<"keep" | "del">(current === "" ? "keep" : "del")
  const [days, setDays] = useState(current)
  const isOwner = membership.role === "owner"
  const value = mode === "keep" ? "" : days
  const save = useMutation({
    mutationFn: () =>
      unwrap(api.PATCH("/v1/workspace", { body: { retention_days: value === "" ? null : Number(value) } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: meKey }),
  })
  const dirty = value !== current
  const n = Number(days)
  const invalid = mode === "del" && (days === "" || !Number.isInteger(n) || n < 1 || n > 36500)
  const saved = membership.workspace.retention_days
  const pick = (next: "keep" | "del") => {
    if (!isOwner) return
    setMode(next)
    if (next === "del" && days === "") setDays("365")
  }
  return (
    <Sec
      id="w-ws"
      title={<Trans>Workspace</Trans>}
      description={<Trans>Shown to your team and in e-mails from Yuva</Trans>}
      pill={
        <Pill tone="muted" data-testid="workspace-retention">
          {saved ? <Days d={saved} /> : <Trans>Keeps everything</Trans>}
        </Pill>
      }
    >
      <form
        className={secCard}
        data-testid="workspace-general"
        onSubmit={(e) => {
          e.preventDefault()
          save.mutate()
        }}
      >
        <SecBody>
          <div className="flex min-w-0 flex-col gap-1.5">
            <Label>
              <Trans>Name</Trans>
            </Label>
            <div className="flex min-h-[38px] flex-wrap items-center gap-x-3 rounded-[9px] border bg-surface ps-3 pe-1.5">
              <span className="min-w-0 flex-1 py-2 [overflow-wrap:anywhere]">{membership.workspace.name}</span>
              <span className="my-1 inline-flex h-[22px] items-center rounded-full bg-muted px-2 text-caption font-medium text-muted-foreground @max-[759px]:hidden">
                <Trans>Set when the workspace was made</Trans>
              </span>
            </div>
            <p className="hidden text-caption text-muted-foreground @max-[759px]:block">
              <Trans>Set when the workspace was made</Trans>
            </p>
          </div>
          <div className="flex min-w-0 flex-col gap-1.5">
            <span id="w-ret-l" className="text-body font-medium">
              <Trans>Closed conversations</Trans>
            </span>
            <div
              role="radiogroup"
              aria-labelledby="w-ret-l"
              className="flex flex-col divide-y overflow-hidden rounded-xl border"
            >
              <RadioRow
                on={mode === "keep"}
                disabled={!isOwner}
                onPick={() => pick("keep")}
                title={<Trans>Keep everything</Trans>}
                hint={<Trans>Conversations, attachments and raw e-mails stay until someone deletes them.</Trans>}
                testId="retention-keep"
              />
              <RadioRow
                on={mode === "del"}
                disabled={!isOwner}
                onPick={() => pick("del")}
                title={<Trans>Delete after a set time</Trans>}
                hint={<Trans>A daily run deletes what nobody touched for that long.</Trans>}
                testId="retention-delete"
              />
              {mode === "del" && (
                <div className="grid grid-cols-[minmax(0,220px)_minmax(0,1fr)] items-start gap-3 bg-brand-wash px-4 pt-1 pb-4 ps-11 @max-[760px]:grid-cols-1 @max-[760px]:ps-4">
                  <div className="flex min-w-0 flex-col gap-1.5">
                    <Label htmlFor="retention-days">
                      <Trans>Delete after (days)</Trans>
                    </Label>
                    <Input
                      id="retention-days"
                      type="number"
                      inputMode="numeric"
                      min={1}
                      max={36500}
                      value={days}
                      disabled={!isOwner}
                      aria-invalid={invalid || undefined}
                      aria-describedby="retention-days-msg"
                      onChange={(e) => setDays(e.target.value)}
                      className="h-[38px] rounded-[9px] border-input bg-card"
                    />
                    <p
                      id="retention-days-msg"
                      className={cn("text-small", invalid ? "text-destructive" : "text-faint")}
                      role={invalid ? "alert" : undefined}
                    >
                      {invalid ? (
                        <Trans>Enter whole days from 1 to 36,500.</Trans>
                      ) : (
                        <Days d={Number.isFinite(n) ? n : 0} />
                      )}
                    </p>
                  </div>
                  <div className="flex min-w-0 flex-col gap-1.5">
                    <span className="text-body font-medium">
                      <Trans>Common choices</Trans>
                    </span>
                    <div className="flex flex-wrap gap-1.5">
                      {PRESETS.map((d) => (
                        <button
                          key={d}
                          type="button"
                          aria-pressed={days === String(d)}
                          disabled={!isOwner}
                          onClick={() => setDays(String(d))}
                          className={cn(
                            days === String(d) ? look.listChipOn : look.listChip,
                            "disabled:cursor-not-allowed disabled:opacity-60"
                          )}
                        >
                          <Days d={d} />
                        </button>
                      ))}
                    </div>
                  </div>
                </div>
              )}
            </div>
          </div>
        </SecBody>
        <SecFoot
          dirty={dirty}
          pending={save.isPending}
          saved={save.isSuccess}
          error={save.error}
          invalid={invalid}
          readOnly={isOwner ? undefined : <Trans>Only owners can change this.</Trans>}
          onDiscard={() => {
            setMode(current === "" ? "keep" : "del")
            setDays(current)
            save.reset()
          }}
        />
      </form>
    </Sec>
  )
}
