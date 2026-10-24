import type { I18n } from "@lingui/core"
import { msg } from "@lingui/core/macro"
import { ai, blog, contact, docsRoot, releases, status, templates, VERSION, type Locale } from "@/lib/routes"

export type Feature = {
  id: string
  visual?: "status"
  shot?: "phone" | "widget" | "broadcast" | "analytics" | "migration"
  title: string
  body: string
  points: { term: string; text: string }[]
  link?: string
  href: string
}

export function homeCopy(i18n: I18n, quickStartRel: string) {
  const version = VERSION
  const locale = i18n.locale as Locale
  const docs = (rel: string) => `${docsRoot(locale)}${rel}`
  const quickStart = docs(quickStartRel)

  return {
    meta: {
      title: i18n._(msg`Yuva: one open-source inbox for e-mail, live chat and in-app messages`),
      description: i18n._(
        msg`Yuva brings support e-mail, live chat and in-app conversations from all of your products into one shared inbox, and adds newsletters, mail migration and cookieless analytics. Open source, self-hosted, one Go binary with Postgres.`
      ),
    },

    hero: {
      title: i18n._(msg`Every support channel, answered as one chat.`),
      lead: i18n._(
        msg`Yuva puts e-mail, live chat and in-app messages from all of your products into one inbox for your team. Open source, one Go binary with Postgres.`
      ),
      signUp: i18n._(msg`Create an account`),
      hosted: i18n._(msg`Hosted`),
      hostedBody: i18n._(msg`Create an account on our service and start without running a server.`),
      selfHosted: i18n._(msg`Self-hosted`),
      selfHostedBody: i18n._(msg`One Docker image and Postgres on your own server.`),
      selfHost: i18n._(msg`Or host it yourself`),
      quickStart: i18n._(msg`Read the quick start`),
      quickStartHref: quickStart,
      github: i18n._(msg`View on GitHub`),
      worksWith: i18n._(msg`Works with`),
      startWithAi: i18n._(msg`Start with AI`),
      startWithAiHref: ai[locale],
      tourLabel: i18n._(msg`Product tour`),
      tabs: [
        { id: "inbox", label: i18n._(msg`Inbox`) },
        { id: "widget", label: i18n._(msg`Channels`) },
        { id: "feedback", label: i18n._(msg`Feedback`) },
        { id: "broadcast", label: i18n._(msg`Broadcasts`) },
        { id: "analytics", label: i18n._(msg`Analytics`) },
        { id: "migration", label: i18n._(msg`Moving in`) },
      ] as { id: "inbox" | "widget" | "feedback" | "broadcast" | "analytics" | "migration"; label: string }[],
      feedbackAlt: i18n._(
        msg`The panel with feedback sent from a mobile app: a bug report with the app version, device and screen, next to the conversation list.`
      ),
      shotAlt: i18n._(
        msg`The Yuva messaging panel: conversations from e-mail, live chat and apps in one list, and a conversation open as a chat, with a team note and a reply an assistant drafted for a teammate to send.`
      ),
    },

    features: {
      guide: i18n._(msg`Read the guide`),
      templateAlt: (name: string) => i18n._(msg`Preview of the ${name} template`),
      appAlts: {
        list: (platform: string) =>
          i18n._(
            msg`The support screen of the ${platform} SDK inside an app: the person's conversations with the latest reply, and a button to write a new message.`
          ),
        thread: (platform: string) =>
          i18n._(
            msg`A conversation in the ${platform} SDK: a question about an order and the team's replies with a tracking link, and a box to write back.`
          ),
      },
      alts: {
        phone: i18n._(
          msg`The panel on a phone: the conversation list with each person's channel and latest message, and the tabs at the bottom.`
        ),
        widget: i18n._(
          msg`The Yuva chat widget open on a newsletter tool's website: the visitor's question and a member's reply, with the member shown online.`
        ),
        broadcast: i18n._(
          msg`The report of a sent newsletter in the panel: 240 sent, 5 unsubscribed and 56 who clicked a link, a chart over time and the number of clicks on each link.`
        ),
        analytics: i18n._(
          msg`The analytics overview in the panel: 30 days of visitors, pageviews and bounce rate, a daily chart with a note on the day a newsletter went out, and the top pages and sources.`
        ),
        migration: i18n._(
          msg`A finished import in the panel: 6 conversations and 12 messages copied from an mbox file, with a note that Yuva has no access to your mailbox.`
        ),
      },
      items: [
        {
          id: "inbox",
          shot: "phone",
          title: i18n._(msg`Inbox and conversations`),
          body: i18n._(msg`Every channel lands in one list. Your team answers from one chat-style view.`),
          points: [
            { term: i18n._(msg`Assign`), text: i18n._(msg`to a teammate`) },
            { term: i18n._(msg`Notes and mentions`), text: i18n._(msg`stay inside the team`) },
            { term: i18n._(msg`Labels and canned replies`), text: i18n._(msg`for the usual answers`) },
          ],
          href: docs("install/"),
        },
        {
          id: "channels",
          shot: "widget",
          title: i18n._(msg`Channels`),
          body: i18n._(msg`Connect a support address, a chat widget for your website or the SDKs for your apps.`),
          points: [
            { term: i18n._(msg`E-mail`), text: i18n._(msg`through your own SMTP account`) },
            { term: i18n._(msg`Live chat`), text: i18n._(msg`with one script tag`) },
            { term: i18n._(msg`In-app`), text: i18n._(msg`with the iOS and Android SDKs`) },
          ],
          href: docs("email/"),
        },
        {
          id: "team-mail",
          title: i18n._(msg`Team mail`),
          body: i18n._(
            msg`Shared addresses like sales@ and people's own mailboxes, in the inbox your team already uses. Yuva is the mailbox: your domain's mail is routed to it. It is not an IMAP server for mail apps.`
          ),
          points: [
            { term: i18n._(msg`Mailboxes`), text: i18n._(msg`shared, or one for each person`) },
            { term: i18n._(msg`Privacy`), text: i18n._(msg`a personal one is private, even from admins`) },
            { term: i18n._(msg`History`), text: i18n._(msg`moved in, read-only at the source`) },
          ],
          href: docs("team-mail/"),
        },
        {
          id: "contacts",
          title: i18n._(msg`Contacts`),
          body: i18n._(msg`One profile for each person, whichever channel they write from.`),
          points: [
            { term: i18n._(msg`History`), text: i18n._(msg`of every conversation`) },
            { term: i18n._(msg`Attributes`), text: i18n._(msg`sent by your own app`) },
            { term: i18n._(msg`Delete`), text: i18n._(msg`a contact and their data`) },
          ],
          href: docs("identity/"),
        },
        {
          id: "team",
          title: i18n._(msg`Team and reports`),
          body: i18n._(msg`See how support is going and who is on the team.`),
          points: [
            { term: i18n._(msg`Today`), text: i18n._(msg`what came in and what is open`) },
            { term: i18n._(msg`Ratings`), text: i18n._(msg`from the people you helped`) },
            { term: i18n._(msg`Members`), text: i18n._(msg`and the inboxes they can see`) },
          ],
          href: docs("configuration/"),
        },
        {
          id: "help",
          title: i18n._(msg`Help center`),
          body: i18n._(msg`Add ratings and a discussion area to your own documentation pages.`),
          points: [
            { term: i18n._(msg`Ratings`), text: i18n._(msg`on every page`) },
            { term: i18n._(msg`Discussions`), text: i18n._(msg`with replies and votes`) },
            { term: i18n._(msg`Accepted answer`), text: i18n._(msg`marked by your team`) },
          ],
          href: docs("documentation-pages/"),
        },
        {
          id: "broadcasts",
          shot: "broadcast",
          title: i18n._(msg`Broadcasts`),
          body: i18n._(msg`Send newsletters to your mailing lists and see what readers do with them.`),
          points: [
            { term: i18n._(msg`Lists`), text: i18n._(msg`with recorded consent`) },
            { term: i18n._(msg`Deliverability`), text: i18n._(msg`checks before you send`) },
            { term: i18n._(msg`Reports`), text: i18n._(msg`with clicks and unsubscribes`) },
          ],
          href: docs("broadcasts/"),
        },
        {
          id: "designs",
          title: i18n._(msg`E-mail templates and designer`),
          body: i18n._(msg`Build e-mails in the designer or start from a free template.`),
          points: [
            { term: i18n._(msg`Designer`), text: i18n._(msg`for your own e-mails`) },
            { term: i18n._(msg`Gallery`), text: i18n._(msg`of free templates`) },
            { term: i18n._(msg`Preview`), text: i18n._(msg`before anything is sent`) },
          ],
          link: i18n._(msg`Open the templates`),
          href: templates[locale],
        },
        {
          id: "analytics",
          shot: "analytics",
          title: i18n._(msg`Analytics`),
          body: i18n._(msg`See who visits your sites and apps, with no cookies and no stored IP addresses.`),
          points: [
            { term: i18n._(msg`Pages and sources`), text: i18n._(msg`at a glance`) },
            { term: i18n._(msg`Goals`), text: i18n._(msg`and funnels`) },
            { term: i18n._(msg`Realtime`), text: i18n._(msg`visitors`) },
          ],
          href: docs("analytics/"),
        },
        {
          id: "status",
          visual: "status",
          title: i18n._(msg`Status pages`),
          body: i18n._(
            msg`A public page for the state of your products, kept in the same inbox as your support. Checks keep it current and subscribers hear by e-mail.`
          ),
          points: [
            { term: i18n._(msg`Components`), text: i18n._(msg`with 90 day bars`) },
            { term: i18n._(msg`Incidents`), text: i18n._(msg`and planned maintenance`) },
            { term: i18n._(msg`Subscribers`), text: i18n._(msg`by e-mail, with double opt-in`) },
          ],
          link: i18n._(msg`Open status pages`),
          href: status[locale],
        },
        {
          id: "assistants",
          title: i18n._(msg`AI assistants and MCP`),
          body: i18n._(msg`Connect Claude, ChatGPT or Cursor. A teammate reviews and sends what they draft.`),
          points: [
            { term: i18n._(msg`Draft replies`), text: i18n._(msg`for open conversations`) },
            { term: i18n._(msg`Designs and broadcast drafts`), text: i18n._(msg`for your newsletters`) },
            { term: i18n._(msg`Analytics`), text: i18n._(msg`read through MCP`) },
          ],
          link: i18n._(msg`Open AI assistants`),
          href: ai[locale],
        },
        {
          id: "moving-in",
          shot: "migration",
          title: i18n._(msg`Moving in`),
          body: i18n._(msg`Bring your mail and support history over. Yuva only reads at the source.`),
          points: [
            { term: i18n._(msg`Mailboxes`), text: i18n._(msg`Gmail, Outlook or any IMAP server`) },
            { term: i18n._(msg`Help desks`), text: i18n._(msg`Zendesk and Help Scout`) },
            { term: i18n._(msg`Files`), text: i18n._(msg`mbox, Takeout and .eml`) },
          ],
          href: docs("migrate/"),
        },
        {
          id: "developers",
          title: i18n._(msg`Developers`),
          body: i18n._(msg`Every part of Yuva has a typed contract and a small, documented integration.`),
          points: [
            { term: "API", text: i18n._(msg`with workspace API keys`) },
            { term: i18n._(msg`Webhooks`), text: i18n._(msg`signed, for every new message`) },
            { term: i18n._(msg`Identity tokens`), text: i18n._(msg`so users write as themselves`) },
          ],
          link: i18n._(msg`Open the API reference`),
          href: docs("api/"),
        },
        {
          id: "self-hosting",
          title: i18n._(msg`Self-hosting and open source`),
          body: i18n._(
            msg`One small server runs all of it. The server is open source, and the SDKs carry no copyleft.`
          ),
          points: [
            { term: i18n._(msg`One binary`), text: i18n._(msg`and Postgres`) },
            { term: "AGPL-3.0", text: i18n._(msg`server and panel`) },
            { term: "MIT", text: i18n._(msg`SDKs and API contract`) },
          ],
          href: quickStart,
        },
      ] as Feature[],
      resources: {
        title: i18n._(msg`Resources`),
        links: [
          { label: i18n._(msg`Documentation`), href: docsRoot(locale) },
          { label: i18n._(msg`Blog`), href: blog[locale] },
          { label: i18n._(msg`Releases`), href: releases[locale] },
          { label: i18n._(msg`Roadmap`), href: docs("roadmap/") },
          { label: i18n._(msg`Contact`), href: contact[locale] },
        ],
      },
    },

    close: {
      title: i18n._(msg`Give every product's users a way to reach you.`),
      lead: i18n._(msg`Read the code, run ${version} on your machine, and follow along as it grows.`),
    },
  }
}

export type HomeCopy = ReturnType<typeof homeCopy>
