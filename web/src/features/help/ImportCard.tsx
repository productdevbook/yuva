import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { LoaderIcon } from "lucide-react"
import { useState } from "react"
import { Link } from "react-router"

import { ProgressBar } from "@/components/common"
import { ExportButtons } from "@/components/common/ExportButtons"
import { Segmented } from "@/components/common/Segmented"
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
import { Switch } from "@/components/ui/switch"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import {
  centerLink,
  useCancelHelpImport,
  useHelpImportRedirects,
  useHelpImports,
  useStartHelpImport,
} from "@/features/help/queries"
import { ErrorLine, useLocaleName } from "@/features/help/text"
import { useMigrations } from "@/features/migrations/queries"
import { Card, ChoiceSelect, Field, Section, ToggleRow } from "@/features/settings/ui"
import { api, type HelpCenter, type HelpImport, type HelpImportCreate } from "@/lib/api"
import { saveDownload } from "@/lib/download"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

const ACTIVE: HelpImport["state"][] = ["queued", "running"]

function useStateText(): Record<HelpImport["state"], string> {
  const { t } = useLingui()
  return {
    queued: t`Waiting to start`,
    running: t`Running`,
    done: t`Done`,
    failed: t`Failed`,
    cancelled: t`Cancelled`,
  }
}

function usePhaseText(): Record<HelpImport["phase"], string> {
  const { t } = useLingui()
  return {
    source: t`Checking the sign-in`,
    collections: t`Writing collections`,
    articles: t`Writing articles and images`,
    links: t`Rewriting links between articles`,
  }
}

const SOURCE_NAMES: Record<HelpImport["source"], string> = { zendesk: "Zendesk Guide", helpscout: "Help Scout Docs" }

function useDate() {
  const { i18n } = useLingui()
  return (iso: string) =>
    new Intl.DateTimeFormat(i18n.locale, { dateStyle: "medium", timeStyle: "short" }).format(new Date(iso))
}

function Counts({ item }: { item: HelpImport }) {
  const c = item.counts
  const saved = c.translations_created + c.translations_updated
  return (
    <ul className="flex flex-wrap gap-x-4 gap-y-1 text-small text-muted-foreground" data-testid="help-import-counts">
      <li>
        <Plural value={c.collections} one="# collection" other="# collections" />
      </li>
      <li>
        <Plural value={c.articles} one="# article read" other="# articles read" />
      </li>
      <li>
        <Plural value={saved} one="# draft saved" other="# drafts saved" />
      </li>
      {c.translations_unchanged > 0 && (
        <li>
          <Plural value={c.translations_unchanged} one="# translation unchanged" other="# translations unchanged" />
        </li>
      )}
      {c.translations_kept > 0 && (
        <li>
          <Plural value={c.translations_kept} one="# edited draft kept" other="# edited drafts kept" />
        </li>
      )}
      <li>
        <Plural value={c.images_stored} one="# image stored" other="# images stored" />
      </li>
      {c.images_failed > 0 && (
        <li className="text-destructive" data-testid="help-import-images-failed">
          <Plural value={c.images_failed} one="# image failed" other="# images failed" />
        </li>
      )}
      {c.links_rewritten > 0 && (
        <li>
          <Plural value={c.links_rewritten} one="# link rewritten" other="# links rewritten" />
        </li>
      )}
      {c.truncated > 0 && (
        <li>
          <Plural
            value={c.truncated}
            one="# body cut at 100,000 characters"
            other="# bodies cut at 100,000 characters"
          />
        </li>
      )}
      {c.failed > 0 && (
        <li className="text-destructive">
          <Plural value={c.failed} one="# item could not be written" other="# items could not be written" />
        </li>
      )}
    </ul>
  )
}

function Skipped({ center, item }: { center: HelpCenter; item: HelpImport }) {
  const name = useLocaleName()
  if (item.skipped_locales.length === 0) return null
  return (
    <div className="flex flex-col gap-1.5" data-testid="help-import-skipped">
      <p className="text-small text-muted-foreground">
        <Plural
          value={item.skipped_locales.length}
          one="The help center does not offer this language, so its translations were not imported."
          other="The help center does not offer these languages, so their translations were not imported."
        />
      </p>
      <ul className="flex flex-wrap gap-2">
        {item.skipped_locales.map((l) => (
          <li key={l} className="flex items-center gap-2 rounded-lg border bg-background px-2.5 py-1 text-body">
            {name(l)}
            <span className="text-caption text-faint uppercase">{l}</span>
            <Link
              to={centerLink(center.id, "settings")}
              className="text-small text-brand underline-offset-4 hover:underline"
            >
              <Trans>Add locale</Trans>
            </Link>
          </li>
        ))}
      </ul>
      <p className="text-small text-faint">
        <Plural
          value={item.skipped_locales.length}
          one="Add the language in the settings, then import again."
          other="Add the languages in the settings, then import again."
        />
      </p>
    </div>
  )
}

function Redirects({ center, item }: { center: HelpCenter; item: HelpImport }) {
  const { t } = useLingui()
  const list = useHelpImportRedirects(center.id, item.id)
  const rows = list.data?.pages.flatMap((p) => p.items) ?? []
  if (item.redirects === 0) return null
  return (
    <div className="flex flex-col gap-2" data-testid="help-import-redirects">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="text-body font-medium">
          <Trans>Redirects</Trans>
        </h3>
        <ExportButtons
          formats={["csv"] as const}
          testId="help-import-csv"
          run={() =>
            saveDownload(
              api.GET("/v1/help-centers/{centerId}/imports/{importId}/redirects.csv", {
                params: { path: { centerId: center.id, importId: item.id } },
                parseAs: "blob",
              }),
              `redirects-${item.id}.csv`
            )
          }
        />
      </div>
      <p className="text-small text-faint">
        <Trans>
          Set these up at the old address yourself. A new address answers “not found” until you publish the article.
        </Trans>
      </p>
      <div className="max-h-72 overflow-auto rounded-xl border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>
                <Trans>Old address</Trans>
              </TableHead>
              <TableHead>
                <Trans>New address</Trans>
              </TableHead>
              <TableHead>
                <Trans>Language</Trans>
              </TableHead>
              <TableHead>
                <Trans>State</Trans>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((r, i) => (
              <TableRow key={`${r.old_url}-${r.locale}-${i}`}>
                <TableCell className="max-w-64 break-all whitespace-normal">{r.old_url}</TableCell>
                <TableCell className="max-w-64 break-all whitespace-normal">{r.new_url}</TableCell>
                <TableCell className="uppercase">{r.locale}</TableCell>
                <TableCell>{r.published ? t`Published` : t`Draft`}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
      <ErrorLine error={list.error} />
      {list.hasNextPage && (
        <Button
          variant="outline"
          size="sm"
          className="self-start"
          disabled={list.isFetchingNextPage}
          onClick={() => void list.fetchNextPage()}
        >
          <Trans>Show more</Trans>
        </Button>
      )}
    </div>
  )
}

function DetailDialog({ center, item, onClose }: { center: HelpCenter; item: HelpImport; onClose: () => void }) {
  const states = useStateText()
  const date = useDate()
  const source = SOURCE_NAMES[item.source]
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-2xl" data-testid="help-import-detail">
        <DialogHeader>
          <DialogTitle>
            <Trans>Import from {source}</Trans>
          </DialogTitle>
          <DialogDescription>
            {states[item.state]} · {date(item.created_at)}
          </DialogDescription>
        </DialogHeader>
        <div className="flex max-h-[70vh] flex-col gap-5 overflow-y-auto">
          {item.error && (
            <p role="alert" className="text-body text-destructive">
              {item.error}
            </p>
          )}
          <Counts item={item} />
          <Skipped center={center} item={item} />
          {item.counts.translations_created + item.counts.translations_updated > 0 && (
            <p className="text-body">
              <Trans>Everything imported is a draft.</Trans>{" "}
              <Link to={centerLink(center.id, "drafts")} className="text-brand underline-offset-4 hover:underline">
                <Trans>Open the drafts</Trans>
              </Link>
            </p>
          )}
          <Redirects center={center} item={item} />
        </div>
        <DialogFooter>
          <Button variant="ghost" onClick={onClose}>
            <Trans>Close</Trans>
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function StartDialog({ center, onClose }: { center: HelpCenter; onClose: () => void }) {
  const { t } = useLingui()
  const name = useLocaleName()
  const { canManage } = useSession()
  const start = useStartHelpImport(center.id)
  const migrations = useMigrations()
  const connected = (migrations.data?.pages.flatMap((p) => p.items) ?? []).filter(
    (m) => m.source === "zendesk" && m.credentials && !m.credentials_deleted_at
  )
  const [source, setSource] = useState<"zendesk" | "helpscout">("zendesk")
  const [picked, setPicked] = useState("")
  const [overwrite, setOverwrite] = useState(false)
  const [key, setKey] = useState("")
  const [site, setSite] = useState("")
  const [locale, setLocale] = useState(center.default_locale)
  const migrationId = connected.some((m) => m.id === picked) ? picked : (connected[0]?.id ?? "")
  const ok = source === "zendesk" ? !!migrationId : key.trim().length > 0
  const submit = () => {
    if (!ok) return
    const body: HelpImportCreate =
      source === "zendesk"
        ? { source, migration_id: migrationId, overwrite_edited: overwrite }
        : {
            source,
            docs_api_key: key.trim(),
            site_id: site.trim() || undefined,
            locale,
            overwrite_edited: overwrite,
          }
    start.mutate(body, { onSuccess: onClose })
  }
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent data-testid="help-import-dialog">
        <form
          className="flex flex-col gap-5"
          onSubmit={(e) => {
            e.preventDefault()
            submit()
          }}
        >
          <DialogHeader>
            <DialogTitle>
              <Trans>Import articles</Trans>
            </DialogTitle>
            <DialogDescription>
              <Trans>
                Brings the collections and articles of another help center in as drafts. Nothing is published and the
                source is only read.
              </Trans>
            </DialogDescription>
          </DialogHeader>
          <Segmented
            value={source}
            onChange={setSource}
            label={t`Import from`}
            items={[
              { value: "zendesk", label: SOURCE_NAMES.zendesk, testId: "help-import-zendesk" },
              { value: "helpscout", label: SOURCE_NAMES.helpscout, testId: "help-import-helpscout" },
            ]}
          />
          {source === "zendesk" ? (
            connected.length > 0 ? (
              <Field
                label={<Trans>Zendesk account</Trans>}
                htmlFor="help-import-migration"
                hint={<Trans>The read-only sign-in of this migration is reused. No new permission is asked.</Trans>}
              >
                <ChoiceSelect
                  id="help-import-migration"
                  value={migrationId}
                  onChange={setPicked}
                  options={connected.map((m) => [m.id, m.source_address] as const)}
                  label={t`Zendesk account`}
                />
              </Field>
            ) : (
              <div className="flex flex-col gap-2 text-body" data-testid="help-import-no-zendesk">
                {canManage ? (
                  <>
                    <p>
                      <Trans>
                        No Zendesk account is signed in. Start a Zendesk migration in Moving to Yuva and sign in; its
                        read-only access is reused here.
                      </Trans>
                    </p>
                    <Link to="/migrations/new" className="self-start text-brand underline-offset-4 hover:underline">
                      <Trans>Start a Zendesk migration</Trans>
                    </Link>
                  </>
                ) : (
                  <p>
                    <Trans>Only owners and admins can use a Zendesk migration.</Trans>
                  </p>
                )}
              </div>
            )
          ) : (
            <>
              <Field
                label={<Trans>Docs API key</Trans>}
                htmlFor="help-import-key"
                hint={
                  <Trans>
                    In Help Scout, open Your Profile, then API Keys, and create a Docs API key. Yuva keeps it sealed
                    until the import ends.
                  </Trans>
                }
              >
                <Input
                  id="help-import-key"
                  type="password"
                  autoComplete="off"
                  spellCheck={false}
                  value={key}
                  onChange={(e) => setKey(e.target.value)}
                  data-testid="help-import-key"
                />
              </Field>
              <div className="grid gap-5 sm:grid-cols-2">
                <Field
                  label={<Trans>Docs site (optional)</Trans>}
                  htmlFor="help-import-site"
                  hint={<Trans>Leave empty to read every site.</Trans>}
                >
                  <Input
                    id="help-import-site"
                    value={site}
                    spellCheck={false}
                    onChange={(e) => setSite(e.target.value)}
                    data-testid="help-import-site"
                  />
                </Field>
                <Field
                  label={<Trans>Language of the articles</Trans>}
                  htmlFor="help-import-locale"
                  hint={<Trans>Docs sites have no languages.</Trans>}
                >
                  <ChoiceSelect
                    id="help-import-locale"
                    value={locale}
                    onChange={setLocale}
                    options={center.locales.map((l) => [l, name(l)] as const)}
                    label={t`Language of the articles`}
                  />
                </Field>
              </div>
            </>
          )}
          <ToggleRow
            title={<Trans>Overwrite edited drafts</Trans>}
            hint={<Trans>Off: a draft you edited in Yuva since the last import is kept.</Trans>}
          >
            <Switch checked={overwrite} onCheckedChange={setOverwrite} data-testid="help-import-overwrite" />
          </ToggleRow>
          <ErrorLine error={start.error} />
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={onClose}>
              <Trans>Cancel</Trans>
            </Button>
            <Button type="submit" disabled={!ok || start.isPending} data-testid="help-import-start">
              {start.isPending && <LoaderIcon className="animate-spin" />}
              <Trans>Start import</Trans>
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function ActiveImport({ center, item }: { center: HelpCenter; item: HelpImport }) {
  const { t } = useLingui()
  const phases = usePhaseText()
  const states = useStateText()
  const cancel = useCancelHelpImport(center.id)
  return (
    <div className="flex flex-col gap-3 rounded-xl border bg-background p-3" data-testid="help-import-active">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-body font-medium">
          {SOURCE_NAMES[item.source]} · {item.state === "queued" ? states.queued : phases[item.phase]}
        </p>
        <Button
          variant="outline"
          size="sm"
          disabled={cancel.isPending}
          onClick={() => cancel.mutate(item.id)}
          data-testid="help-import-cancel"
        >
          <Trans>Cancel import</Trans>
        </Button>
      </div>
      <ProgressBar label={t`Import progress`} />
      <Counts item={item} />
      <ErrorLine error={cancel.error} />
    </div>
  )
}

export function ImportCard({ center }: { center: HelpCenter }) {
  const list = useHelpImports(center.id)
  const states = useStateText()
  const date = useDate()
  const [starting, setStarting] = useState(false)
  const [openId, setOpenId] = useState<string>()
  const items = list.data?.pages.flatMap((p) => p.items) ?? []
  const active = items.find((i) => ACTIVE.includes(i.state))
  const past = items.filter((i) => i !== active)
  const open = items.find((i) => i.id === openId)
  return (
    <Section title={<Trans>Import articles</Trans>}>
      <Card>
        <div className="flex flex-wrap items-center gap-3">
          <p className="min-w-0 flex-1 text-body">
            <Trans>
              Bring in the articles of a Zendesk Guide or Help Scout Docs site, with translations and images, as drafts.
            </Trans>
          </p>
          <Button size="sm" disabled={!!active} onClick={() => setStarting(true)} data-testid="help-import-open">
            <Trans>Import articles</Trans>
          </Button>
        </div>
        {active && <ActiveImport center={center} item={active} />}
        {past.length > 0 && (
          <ul className="flex flex-col divide-y rounded-xl border" data-testid="help-import-history">
            {past.map((i) => (
              <li key={i.id}>
                <button
                  type="button"
                  onClick={() => setOpenId(i.id)}
                  className="flex w-full flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2.5 text-start text-body hover:bg-muted/50"
                >
                  <span className="font-medium">{SOURCE_NAMES[i.source]}</span>
                  <span
                    className={cn("text-small", i.state === "failed" ? "text-destructive" : "text-muted-foreground")}
                  >
                    {states[i.state]}
                  </span>
                  <span className="ms-auto text-small text-faint">{date(i.created_at)}</span>
                </button>
              </li>
            ))}
          </ul>
        )}
        <ErrorLine error={list.error} />
        {list.hasNextPage && (
          <Button variant="outline" size="sm" className="self-start" onClick={() => void list.fetchNextPage()}>
            <Trans>Show older imports</Trans>
          </Button>
        )}
      </Card>
      {starting && <StartDialog center={center} onClose={() => setStarting(false)} />}
      {open && <DetailDialog center={center} item={open} onClose={() => setOpenId(undefined)} />}
    </Section>
  )
}
