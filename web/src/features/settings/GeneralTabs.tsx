import { TopPortal } from "@/components/common"
import { useSettingsAbout, useSettingsNames, type SettingsPlace } from "@/features/settings/SettingsFrame"
import { PageHeader } from "@/features/settings/ui"

export function GeneralHeader({ current, plain }: { current: SettingsPlace; plain?: boolean }) {
  const names = useSettingsNames()
  const about = useSettingsAbout()
  return <PageHeader back={false} title={names.place[current]} description={!plain && about[current]} />
}

export function Intro({ children, action }: { children: React.ReactNode; action?: React.ReactNode }) {
  return (
    <>
      <p className="m-0 max-w-[680px] text-body text-muted-foreground">{children}</p>
      {action && <TopPortal>{action}</TopPortal>}
    </>
  )
}
