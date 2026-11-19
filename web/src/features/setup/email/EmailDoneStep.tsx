import { Trans, useLingui } from "@lingui/react/macro"
import { PartyPopperIcon } from "lucide-react"
import { Link, Navigate, useLocation } from "react-router"

import { StepHeader, SummaryList } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { channelLink, useDeliverability } from "@/features/settings/channels/email/deliverability"
import { isSesHost } from "@/features/settings/channels/email/form"
import { useInbox } from "@/features/settings/inboxes/queries"
import { sesRegion } from "@/features/setup/email/draft"
import { BackHome, useFromHome } from "@/features/setup/BackHome"
import { EmailLayout } from "@/features/setup/email/EmailLayout"
import { useChannel } from "@/features/setup/InstallStep"
import { MailMock, PreviewCaption } from "@/features/setup/previews"
import { type SetupState } from "@/features/setup/setup"

export function EmailDoneStep({ inboxId, channelId }: { inboxId: string; channelId: string }) {
  const { t } = useLingui()
  const state = (useLocation().state ?? {}) as SetupState
  const channel = useChannel(channelId).data
  const inbox = useInbox(inboxId).data
  const email = channel?.email
  const name = inbox?.name ?? ""
  const check = useDeliverability(email?.smtp ? channelId : undefined).data
  if (channel && !email) return <Navigate to={`/setup/${inboxId}/${channelId}`} replace state={state} />
  const host = email?.smtp?.host ?? ""
  const region = sesRegion(host)
  const via = !email?.smtp
    ? t`Only receives`
    : isSesHost(host)
      ? region
        ? t`Amazon SES · ${region}`
        : t`Amazon SES`
      : host
  const sesState =
    email?.ses &&
    {
      isolated: t`Own tenant`,
      pending: t`Setting up…`,
      identity_missing: t`Domain not verified`,
      paused: t`Paused`,
      not_isolated: t`Not isolated`,
      error: t`Error`,
    }[email.ses.state]
  const verdict = !email?.smtp
    ? null
    : !check
      ? t`Not checked`
      : {
          queued: t`Checking…`,
          sent: t`Checking…`,
          received: t`Checked`,
          unverifiable: t`DNS only`,
          timeout: t`No answer`,
          failed: t`Failed`,
        }[check.state]
  const dkim = email?.dkim
  const items: [React.ReactNode, React.ReactNode][] = email
    ? [
        [<Trans key="a">Support address</Trans>, email.address],
        ...(channel.forwarding_address
          ? [
              [<Trans key="f">Forwarding address</Trans>, channel.forwarding_address] as [
                React.ReactNode,
                React.ReactNode,
              ],
            ]
          : []),
        [<Trans key="v">Replies via</Trans>, via],
        ...(sesState ? [[<Trans key="s">Amazon SES</Trans>, sesState] as [React.ReactNode, React.ReactNode]] : []),
        ...(verdict ? [[<Trans key="c">Last check</Trans>, verdict] as [React.ReactNode, React.ReactNode]] : []),
        ...(email.smtp
          ? [
              [
                <Trans key="d">DKIM</Trans>,
                dkim ? (dkim.enabled ? t`Signing` : t`Not signing yet`) : t`Not set up`,
              ] as [React.ReactNode, React.ReactNode],
            ]
          : []),
      ]
    : []
  const toList = state.back?.startsWith("/setup/newsletter") ? state.back : undefined
  const fromHome = useFromHome()
  const back = { back: `/settings/inboxes/${inboxId}` }
  return (
    <EmailLayout
      stage="done"
      isNew={state.existing === false}
      inboxId={inboxId}
      channelId={channelId}
      preview={
        <>
          <MailMock name={inbox?.name ?? ""} to={email?.address ?? ""} />
          <PreviewCaption>
            <Trans>Each new e-mail starts a conversation; replies continue it.</Trans>
          </PreviewCaption>
        </>
      }
    >
      <StepHeader icon={<PartyPopperIcon />} title={<Trans>The e-mail channel is ready</Trans>}>
        <Trans>{name} receives mail at this address. Here is what you set up.</Trans>
      </StepHeader>
      {!email ? (
        <Skeleton className="h-24 w-full" />
      ) : (
        <div className="flex flex-col gap-8" data-testid="setup-email-done">
          <SummaryList items={items} />
          <div className="flex flex-wrap gap-2">
            {toList && (
              <Button size="lg" render={<Link to={toList} replace />} data-testid="setup-back-to-origin">
                <Trans>Back to your list</Trans>
              </Button>
            )}
            <BackHome />
            <Button
              size="lg"
              variant={toList || fromHome ? "outline" : "default"}
              render={<Link to={`/settings/inboxes/${inboxId}`} />}
              data-testid="setup-open-inbox"
            >
              <Trans>Open the inbox</Trans>
            </Button>
            {!email.smtp && (
              <Button
                size="lg"
                variant="outline"
                render={<Link to={channelLink(inboxId, channelId, "replies")} state={back} />}
                data-testid="setup-add-replies"
              >
                <Trans>Add a reply account</Trans>
              </Button>
            )}
            {email.smtp && !dkim?.enabled && !email.ses && (
              <Button
                size="lg"
                variant="outline"
                render={<Link to={channelLink(inboxId, channelId, "dkim")} state={back} />}
                data-testid="setup-set-up-dkim"
              >
                <Trans>Set up DKIM</Trans>
              </Button>
            )}
            <Button size="lg" variant="outline" render={<Link to="/setup/newsletter" state={back} />}>
              <Trans>Start a mailing list</Trans>
            </Button>
            <Button size="lg" variant="outline" render={<Link to={`/migrations/new?inbox=${inboxId}`} state={back} />}>
              <Trans>Bring your old mail</Trans>
            </Button>
          </div>
        </div>
      )}
    </EmailLayout>
  )
}
