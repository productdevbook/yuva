import { Plural, Trans, useLingui } from "@lingui/react/macro"
import {
  ArrowRightIcon,
  ArrowUpRightIcon,
  CircleCheckIcon,
  InboxIcon,
  KeyRoundIcon,
  LoaderIcon,
  ShieldCheckIcon,
} from "lucide-react"
import { Link, useNavigate } from "react-router"

import { inboxRoot } from "@/app/shell"
import { ErrorLine, Notice, ProgressBar, StepHeader } from "@/components/common"
import { formatDateTime } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { useNumber } from "@/features/broadcasts/BroadcastParts"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { Figure, SkippedList, useMigrationConfirms, useStateAnnouncement } from "@/features/migrations/parts"
import { useCreateMigration, useMigrations } from "@/features/migrations/queries"
import { isHelpdesk, isPersonalMicrosoft, useGuides } from "@/features/migrations/text"
import { WizardLayout } from "@/features/migrations/Wizard"
import type { Migration, MigrationPreset } from "@/lib/api"
import { useSession } from "@/lib/session"

export function DoneStep({ m, preset }: { m: Migration; preset: MigrationPreset }) {
  const { t, i18n } = useLingui()
  const num = useNumber()
  const navigate = useNavigate()
  const { canManage } = useSession()
  const [, setInbox] = useInboxFilter()
  const guide = useGuides()[m.preset]
  const ask = useMigrationConfirms(m)
  const announcement = useStateAnnouncement([m])
  const address = m.source_address
  const provider = preset.name
  const c = m.counts
  const est = m.estimate
  const removing = m.state === "removing"
  const removed = m.state === "removed"
  const completed = m.state === "completed"
  const deletedAt = m.credentials_deleted_at ? formatDateTime(m.credentials_deleted_at, i18n.locale) : ""
  const removable = m.removable_conversations ?? 0
  const file = m.source === "file"
  const desk = isHelpdesk(m.source)
  const fileDeletedAt = m.upload?.deleted_at ? formatDateTime(m.upload.deleted_at, i18n.locale) : ""
  const openInbox = () => {
    if (m.inbox_id) setInbox(m.inbox_id)
    navigate(inboxRoot(canManage))
  }
  return (
    <WizardLayout step="done" m={m} preset={preset}>
      <StepHeader
        icon={completed ? <CircleCheckIcon /> : undefined}
        title={
          completed && desk ? (
            <Trans>The history from {provider} is in Yuva</Trans>
          ) : completed ? (
            <Trans>{address} is in Yuva</Trans>
          ) : removing ? (
            <Trans>Removing imported history…</Trans>
          ) : removed ? (
            <Trans>Imported history removed</Trans>
          ) : (
            <Trans>Migration cancelled</Trans>
          )
        }
      >
        {completed && file ? (
          <Trans>The history from the export file is copied into Yuva.</Trans>
        ) : completed ? (
          <Trans>The history is copied and its mail now reaches Yuva.</Trans>
        ) : removed && desk ? (
          <Trans>Conversations nobody wrote in after the import are deleted. Nothing changed in {provider}.</Trans>
        ) : removed ? (
          <Trans>Conversations nobody wrote in after the import are deleted. Nothing changed in {address}.</Trans>
        ) : removing && desk ? (
          <Trans>Conversations your team wrote in stay. Nothing changes in {provider}.</Trans>
        ) : removing ? (
          <Trans>Conversations your team wrote in stay. Nothing changes in {address}.</Trans>
        ) : desk ? (
          <Trans>What was imported stays. Nothing changed in {provider}.</Trans>
        ) : (
          <Trans>What was imported stays. Nothing changed in {address}.</Trans>
        )}
      </StepHeader>
      {announcement}

      <div className="flex flex-col gap-8">
        {removing && <ProgressBar label={t`Removing imported history`} text={t`Removing`} />}

        {!removed && (
          <section className="flex flex-col gap-3" aria-labelledby="migration-summary">
            <h2 id="migration-summary" className="text-title">
              <Trans>What happened</Trans>
            </h2>
            <div className="grid grid-cols-2 gap-2.5" data-testid="migration-summary">
              <Figure value={num(m.imported_conversations ?? 0)} label={<Trans>Conversations</Trans>} />
              {!desk && <Figure value={num(c.imported)} label={<Trans>Messages imported</Trans>} />}
              <Figure value={num(c.duplicate)} label={<Trans>Already in Yuva</Trans>} />
              <Figure value={num(c.skipped + c.failed)} label={<Trans>Skipped or failed</Trans>} />
              {est && <Figure value={num(est.contacts_new)} label={<Trans>New contacts</Trans>} />}
              {est && <Figure value={num(est.labels.length)} label={<Trans>Labels</Trans>} />}
            </div>
            {est && (
              <>
                <h3 className="mt-2 text-body font-medium">
                  <Trans>Not imported</Trans>
                </h3>
                <SkippedList skipped={est.skipped} />
              </>
            )}
          </section>
        )}

        {completed && m.preset === "takeout" && <NewerMail m={m} />}

        {desk ? (
          <section className="flex flex-col gap-3" aria-labelledby="migration-access">
            <h2 id="migration-access" className="text-title">
              <Trans>Access to {provider}</Trans>
            </h2>
            <p className="flex items-start gap-2 text-body" data-testid="migration-credentials">
              <ShieldCheckIcon className="mt-0.5 size-4 shrink-0 text-success" />
              <span>
                {deletedAt ? (
                  <Trans>
                    Yuva deleted its token {deletedAt} and can no longer read {provider}.
                  </Trans>
                ) : (
                  <Trans>Yuva deleted its token and can no longer read {provider}.</Trans>
                )}
              </span>
            </p>
            <Notice icon={KeyRoundIcon} data-testid="migration-revoke-note">
              <p className="font-medium text-foreground">
                {m.source === "zendesk" ? (
                  <Trans>Delete the OAuth client you created for Yuva</Trans>
                ) : m.source === "front" ? (
                  <Trans>Delete the API token you created for Yuva</Trans>
                ) : m.source === "freshdesk" ? (
                  <Trans>Reset the API key you gave Yuva</Trans>
                ) : m.source === "crisp" ? (
                  <Trans>Roll the website token you created for Yuva</Trans>
                ) : (
                  <Trans>Delete the app you created for Yuva</Trans>
                )}
              </p>
              <p>
                {m.source === "crisp" ? (
                  <Trans>
                    Yuva deleted its token but cannot revoke it in Crisp. Regenerate the token in Settings › Workspace
                    Settings › Advanced configuration; Yuva does not need it any more.
                  </Trans>
                ) : m.source === "zendesk" ? (
                  <Trans>
                    Yuva deleted its token but cannot revoke it in Zendesk. Delete the client in Admin Center › Apps and
                    integrations › APIs › OAuth clients; Yuva does not need it any more.
                  </Trans>
                ) : m.source === "front" ? (
                  <Trans>
                    Yuva deleted its copy of the token but cannot revoke it in Front. Delete the token in Settings ›
                    Developers › API tokens; Yuva does not need it any more.
                  </Trans>
                ) : m.source === "freshdesk" ? (
                  <Trans>
                    Yuva deleted its key but cannot reset it in Freshdesk. Open Profile settings and choose Reset API
                    key; Yuva does not need it any more.
                  </Trans>
                ) : (
                  <Trans>
                    Yuva deleted its token but cannot revoke it in Help Scout. Delete the app in Your Profile › My Apps;
                    Yuva does not need it any more.
                  </Trans>
                )}
              </p>
              <Button
                variant="outline"
                size="sm"
                className="self-start"
                render={<a href={guide.link} target="_blank" rel="noreferrer" />}
                data-testid="migration-revoke"
              >
                {guide.linkLabel}
                <ArrowUpRightIcon className="rtl:-scale-x-100" />
              </Button>
            </Notice>
            {completed && (
              <ul className="flex list-disc flex-col gap-1 ps-5 text-body text-muted-foreground">
                <li>
                  <Trans>Keep the forwarding, routing or MX as it is: that is how mail reaches Yuva now.</Trans>
                </li>
                <li>
                  <Trans>Nothing changed in {provider}; keep the account as an archive if you like.</Trans>
                </li>
              </ul>
            )}
          </section>
        ) : file ? (
          <section className="flex flex-col gap-3" aria-labelledby="migration-file">
            <h2 id="migration-file" className="text-title">
              <Trans>The export file</Trans>
            </h2>
            <p className="flex items-start gap-2 text-body" data-testid="migration-file-deleted">
              <ShieldCheckIcon className="mt-0.5 size-4 shrink-0 text-success" />
              <span>
                {fileDeletedAt ? (
                  <Trans>The uploaded file was deleted {fileDeletedAt}. Yuva keeps only what it imported.</Trans>
                ) : (
                  <Trans>The uploaded file is deleted when the migration ends. Yuva keeps only what it imported.</Trans>
                )}
              </span>
            </p>
          </section>
        ) : (
          <section className="flex flex-col gap-3" aria-labelledby="migration-access">
            <h2 id="migration-access" className="text-title">
              <Trans>Access to the mailbox</Trans>
            </h2>
            <p className="flex items-start gap-2 text-body" data-testid="migration-credentials">
              <ShieldCheckIcon className="mt-0.5 size-4 shrink-0 text-success" />
              <span>
                {deletedAt ? (
                  <Trans>
                    Credentials deleted {deletedAt}. Yuva can no longer read {address}.
                  </Trans>
                ) : (
                  <Trans>Credentials deleted. Yuva can no longer read {address}.</Trans>
                )}
              </span>
            </p>
            {m.preset === "microsoft" ? (
              <MicrosoftAccess address={address} team={preset.team_client === true} />
            ) : (
              <>
                <p className="text-body text-muted-foreground">
                  {preset.app_password ? (
                    <Trans>Remove the app password you made for Yuva in {provider}; it is not needed any more.</Trans>
                  ) : (
                    <Trans>If you made a password just for Yuva, remove it in {provider}.</Trans>
                  )}
                </p>
                <Button
                  variant="outline"
                  size="sm"
                  className="self-start"
                  render={<a href={guide.link} target="_blank" rel="noreferrer" />}
                  data-testid="migration-revoke"
                >
                  {guide.linkLabel}
                  <ArrowUpRightIcon className="rtl:-scale-x-100" />
                </Button>
              </>
            )}
            {completed && (
              <ul className="flex list-disc flex-col gap-1 ps-5 text-body text-muted-foreground">
                <li>
                  <Trans>Keep the forwarding, routing or MX as it is: that is how mail reaches Yuva now.</Trans>
                </li>
                <li>
                  <Trans>The old mailbox is untouched; keep it as an archive if you like.</Trans>
                </li>
              </ul>
            )}
          </section>
        )}

        <ErrorLine error={ask.error} />
        <div className="flex flex-wrap items-center gap-2">
          {m.inbox_id && (
            <Button onClick={openInbox} data-testid="migration-open-inbox">
              <InboxIcon />
              <Trans>Open the inbox</Trans>
            </Button>
          )}
          {m.actions.includes("remove_history") && (
            <Button
              variant="ghost"
              className="text-destructive"
              onClick={ask.removeHistory}
              disabled={ask.pending}
              data-testid="migration-remove-history"
            >
              <Trans>Remove imported history</Trans>
            </Button>
          )}
        </div>
        {m.actions.includes("remove_history") && (
          <p className="-mt-5 text-small text-faint">
            <Plural
              value={removable}
              one="# imported conversation nobody wrote in since can be removed."
              other="# imported conversations nobody wrote in since can be removed."
            />
          </p>
        )}
        {ask.dialog}
      </div>
    </WizardLayout>
  )
}

function MicrosoftAccess({ address, team }: { address: string; team: boolean }) {
  const personal = isPersonalMicrosoft(address)
  const link = (href: string, label: React.ReactNode, testId?: string) => (
    <Button
      variant="outline"
      size="sm"
      render={<a href={href} target="_blank" rel="noreferrer" />}
      data-testid={testId}
    >
      {label}
      <ArrowUpRightIcon className="rtl:-scale-x-100" />
    </Button>
  )
  return (
    <Notice icon={KeyRoundIcon} data-testid="migration-revoke-note">
      <p className="font-medium text-foreground">
        <Trans>Remove Yuva's access in Microsoft</Trans>
      </p>
      <p>
        {personal ? (
          <Trans>
            Yuva deleted its token but cannot revoke it at Microsoft. In your Microsoft account, open Privacy › Apps and
            services, choose Yuva and remove its permissions.
          </Trans>
        ) : (
          <Trans>
            Yuva deleted its token but cannot revoke it at Microsoft. Remove Yuva from your work account in My Apps; an
            admin removes it for everyone in the Microsoft Entra admin center › Enterprise applications.
          </Trans>
        )}
      </p>
      {team && (
        <p>
          <Trans>
            The app registration you created for Yuva is not needed any more either: delete it in Microsoft Entra › App
            registrations.
          </Trans>
        </p>
      )}
      <div className="flex flex-wrap gap-2">
        {personal
          ? link(
              "https://account.microsoft.com/privacy/app-access",
              <Trans>Open Apps and services</Trans>,
              "migration-revoke"
            )
          : link("https://myapps.microsoft.com", <Trans>Open My Apps</Trans>, "migration-revoke")}
        {!personal &&
          link(
            "https://entra.microsoft.com/#view/Microsoft_AAD_IAM/StartboardApplicationsMenuBlade/~/AppAppsPreview",
            <Trans>Open Enterprise applications</Trans>
          )}
        {team &&
          link(
            "https://entra.microsoft.com/#view/Microsoft_AAD_RegisteredApps/ApplicationsListBlade",
            <Trans>Open App registrations</Trans>
          )}
      </div>
    </Notice>
  )
}

function NewerMail({ m }: { m: Migration }) {
  const navigate = useNavigate()
  const create = useCreateMigration()
  const list = useMigrations()
  const next = list.data?.pages.flatMap((p) => p.items).find((x) => x.follows_migration_id === m.id)
  const address = m.source_address
  const domain = address.split("@")[1]?.toLowerCase() ?? ""
  const preset = domain === "gmail.com" || domain === "googlemail.com" ? "gmail" : "google_workspace"
  const start = () =>
    create.mutate(
      {
        preset,
        source_address: address,
        ...(m.inbox_id ? { inbox_id: m.inbox_id } : {}),
        ...(m.channel_id ? { channel_id: m.channel_id } : {}),
      },
      { onSuccess: (n) => navigate(`/migrations/${n.id}/connect`) }
    )
  return (
    <section
      className="flex flex-col gap-3 rounded-2xl border bg-card p-4"
      aria-labelledby="migration-newer"
      data-testid="migration-newer-mail"
    >
      <h2 id="migration-newer" className="text-title">
        <Trans>Newer mail and the switch</Trans>
      </h2>
      <p className="text-body text-muted-foreground">
        <Trans>
          The file holds the mail up to the day you exported it. Connect {address} with an app password: Yuva reads only
          the mail since then, then helps you switch the mailbox's mail over to Yuva.
        </Trans>
      </p>
      <ErrorLine error={create.error} />
      {next ? (
        <Button
          className="self-start"
          render={<Link to={`/migrations/${next.id}`} />}
          data-testid="migration-newer-open"
        >
          <Trans>Continue with the newer mail</Trans>
          <ArrowRightIcon className="rtl:rotate-180" />
        </Button>
      ) : (
        <Button
          className="self-start"
          onClick={start}
          disabled={create.isPending || list.isPending}
          data-testid="migration-newer-start"
        >
          {create.isPending && <LoaderIcon className="animate-spin" />}
          <Trans>Connect {address}</Trans>
          <ArrowRightIcon className="rtl:rotate-180" />
        </Button>
      )}
    </section>
  )
}
