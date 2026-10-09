import { Trans, useLingui } from "@lingui/react/macro"
import { MailIcon, MessageCircleIcon, SmartphoneIcon } from "lucide-react"

import { YuvaMark } from "@/components/common"
import { LanguageMenu } from "@/components/common/LanguageMenu"
import { useVersion } from "@/lib/api"

export function AuthLayout({ children, wide }: { children: React.ReactNode; wide?: boolean }) {
  const version = useVersion().data?.version
  return (
    <div className="grid min-h-svh bg-background lg:grid-cols-[minmax(0,1fr)_minmax(0,1.05fr)]">
      <div className="flex min-w-0 flex-col px-5 sm:px-8">
        <header className="flex h-16 shrink-0 items-center justify-between gap-4">
          <span className="flex items-center gap-2.5">
            <YuvaMark className="size-[26px]" />
            <span className="text-title">Yuva</span>
          </span>
          <LanguageMenu />
        </header>
        <main className="flex flex-1 items-start justify-center pt-[8vh] pb-10 sm:items-center sm:py-16">
          <div className={wide ? "w-full max-w-[26rem]" : "w-full max-w-[22rem]"}>{children}</div>
        </main>
        <footer className="flex h-14 shrink-0 items-center text-caption text-faint">
          {version && <Trans>Yuva {version}</Trans>}
        </footer>
      </div>
      <BrandPanel />
    </div>
  )
}

const rows = [
  { name: "Priya Raman", product: "Fieldnote", dot: "bg-emerald-500", icon: SmartphoneIcon, unread: true },
  { name: "Lina Haddad", product: "Paperboat", dot: "bg-sky-500", icon: MessageCircleIcon, unread: true },
  { name: "Amara Okafor", product: "Tally", dot: "bg-violet-500", icon: MailIcon, unread: false },
] as const

function BrandPanel() {
  const { t } = useLingui()
  const text = [
    { channel: t`In-app`, subject: t`Notes stopped syncing after the update` },
    { channel: t`Live chat`, subject: t`Can I schedule a newsletter for each subscriber?` },
    { channel: t`E-mail`, subject: t`Custom domain still shows as unverified` },
  ]
  return (
    <aside className="hidden p-3 lg:flex" aria-hidden="true">
      <div className="flex flex-1 flex-col justify-between gap-12 overflow-hidden rounded-[1.75rem] bg-surface p-12">
        <p className="flex items-center gap-2 text-body text-muted-foreground">
          <span className="size-1.5 rounded-full bg-brand" />
          <Trans>E-mail, live chat and in-app messages</Trans>
        </p>
        <div className="mx-auto w-full max-w-md overflow-hidden rounded-2xl border bg-card shadow-[0_1px_2px_rgb(15_23_42/0.04),0_24px_60px_-28px_rgb(15_23_42/0.35)]">
          <div className="flex items-center justify-between border-b px-5 py-3.5">
            <p className="text-body font-medium">
              <Trans>One inbox</Trans>
            </p>
            <ul className="flex items-center gap-3">
              {rows.map((r) => (
                <li key={r.product} className="flex items-center gap-1.5 text-caption text-muted-foreground">
                  <span className={`size-1.5 rounded-full ${r.dot}`} />
                  {r.product}
                </li>
              ))}
            </ul>
          </div>
          <ul className="divide-y">
            {rows.map((r, i) => (
              <li key={r.name} className="flex items-start gap-3.5 px-5 py-4">
                <span className="grid size-9 shrink-0 place-items-center rounded-full border text-muted-foreground">
                  <r.icon className="size-4" />
                </span>
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2 text-body">
                    <span className="truncate font-medium">{r.name}</span>
                    <span className="flex shrink-0 items-center gap-1.5 text-muted-foreground">
                      <span className={`size-1.5 rounded-full ${r.dot}`} />
                      {r.product}
                    </span>
                    <span className="ms-auto shrink-0 text-caption text-muted-foreground">{text[i]!.channel}</span>
                    {r.unread && <span className="size-2 shrink-0 rounded-full bg-brand" />}
                  </div>
                  <p className="mt-1 truncate text-body text-muted-foreground">{text[i]!.subject}</p>
                </div>
              </li>
            ))}
          </ul>
        </div>
        <div className="max-w-md">
          <p className="text-page text-balance">
            <Trans>Every customer conversation, in one place.</Trans>
          </p>
          <p className="mt-3 text-muted-foreground">
            <Trans>Reply, assign and follow up together, across all of your products.</Trans>
          </p>
        </div>
      </div>
    </aside>
  )
}
