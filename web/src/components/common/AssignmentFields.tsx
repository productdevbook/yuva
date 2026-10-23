import { Trans, useLingui } from "@lingui/react/macro"

import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { ChoiceSelect } from "@/features/settings/ui"
import type { AssignmentMode } from "@/lib/api"

export const CAP_MAX = 500

export function capValid(cap: string) {
  if (cap === "") return true
  const n = Number(cap)
  return Number.isInteger(n) && n >= 1 && n <= CAP_MAX
}

export function AssignmentFields({
  scope,
  mode,
  cap,
  disabled,
  onMode,
  onCap,
}: {
  scope: "inbox" | "team"
  mode: AssignmentMode
  cap: string
  disabled?: boolean
  onMode: (m: AssignmentMode) => void
  onCap: (c: string) => void
}) {
  const { t } = useLingui()
  const options: [AssignmentMode, string][] = [
    ["off", t`Off`],
    ["round_robin", t`Round robin`],
    ["least_open", t`Fewest open conversations`],
  ]
  return (
    <div className="flex flex-col gap-4" data-testid="assignment-fields">
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={`${scope}-assignment-mode`}>
          <Trans>Automatic assignment</Trans>
        </Label>
        <ChoiceSelect
          id={`${scope}-assignment-mode`}
          label={t`Automatic assignment`}
          value={mode}
          options={options}
          disabled={disabled}
          onChange={onMode}
          className="max-w-72"
        />
        <p className="text-caption text-faint">
          {scope === "team" ? (
            <Trans>Used for conversations given to this team without a member. Only team members are considered.</Trans>
          ) : (
            <Trans>
              Gives a conversation without an assignee to a teammate when it arrives or is reopened by the contact.
              Automation rules run first.
            </Trans>
          )}
        </p>
      </div>
      {mode !== "off" && (
        <>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor={`${scope}-assignment-cap`}>
              <Trans>Most open conversations per member</Trans>
            </Label>
            <Input
              id={`${scope}-assignment-cap`}
              type="number"
              inputMode="numeric"
              min={1}
              max={CAP_MAX}
              value={cap}
              disabled={disabled}
              placeholder={t`No limit`}
              aria-invalid={!capValid(cap) || undefined}
              onChange={(e) => onCap(e.target.value)}
              className="h-9 max-w-40 rounded-[10px] bg-background"
            />
            <p className="text-caption text-faint">
              <Trans>
                Between 1 and 500, counted across the workspace. Assigning by hand ignores it. Leave empty for no limit.
              </Trans>
            </p>
          </div>
          <p className="text-caption text-faint">
            <Trans>
              A member is eligible while they have access to the product (owners and admins only with a grant), have the
              panel or an app open, are set to Auto availability, the product is inside its business hours and they are
              under the limit. When nobody is eligible the conversation waits and goes to the first teammate who becomes
              available.
            </Trans>
          </p>
        </>
      )}
    </div>
  )
}
