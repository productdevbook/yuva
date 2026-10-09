import { Trans, useLingui } from "@lingui/react/macro"

import { PLATFORMS, useEnumText } from "@/components/common/text"
import { Checkbox } from "@/components/ui/checkbox"
import { Switch } from "@/components/ui/switch"
import type { AppForm } from "@/features/settings/channels/app/form"
import { FieldError, FormBlock } from "@/features/settings/channels/parts"
import { ToggleRow } from "@/features/settings/ui"
import { Label } from "@/components/ui/label"

export function AppFields({ f, set }: { f: AppForm; set: (patch: Partial<AppForm>) => void }) {
  const { t } = useLingui()
  const text = useEnumText()
  const none = f.platforms.length === 0
  return (
    <FormBlock title={<Trans>Mobile app</Trans>}>
      <div className="flex flex-col gap-2">
        <span className="text-body font-medium" id="app-platforms">
          <Trans>Platforms</Trans>
        </span>
        <div className="flex gap-5" role="group" aria-labelledby="app-platforms">
          {PLATFORMS.map((p) => (
            <Label key={p} className="font-normal">
              <Checkbox
                checked={f.platforms.includes(p)}
                onCheckedChange={(on) => set({ platforms: on ? [...f.platforms, p] : f.platforms.filter((x) => x !== p) })}
                aria-invalid={(none && f.touched) || undefined}
              />
              {text.platform[p]}
            </Label>
          ))}
        </div>
        <FieldError id="err-app-platforms">{none && <Trans>Pick at least one platform.</Trans>}</FieldError>
      </div>
      <ToggleRow
        title={<Trans>Allow anonymous users</Trans>}
        hint={<Trans>People who are not signed in to your app can write without an identity token from your backend.</Trans>}
      >
        <Switch checked={f.allowAnonymous} onCheckedChange={(allowAnonymous) => set({ allowAnonymous })} aria-label={t`Allow anonymous users`} />
      </ToggleRow>
    </FormBlock>
  )
}
