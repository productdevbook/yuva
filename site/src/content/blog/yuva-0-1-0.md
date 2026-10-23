---
title: "Yuva 0.1.0: teams, reports, single sign-on and five more help desks"
description: "Yuva 0.1.0 adds teams with assignment and response-time targets, Reports, Slack and issue-tracker links, push sent by Yuva, single sign-on for members and history imports from five more help desks. It also changes four things you must handle when you upgrade."
draft: true
author: productdevbook
---

Yuva is an open-source inbox for every product you run. E-mail, live chat on a website and in-app conversations from iOS and Android land in one place, next to feedback, a help center, roadmap and status pages, newsletters and cookieless analytics. You can use our hosted service or run the same code yourself with Postgres and one container. After the [mobile screens](/blog/yuva-mobile-sdks/) and more than thirty small releases, [0.1.0](https://github.com/productdevbook/yuva/releases/tag/v0.1.0) is the first one we number as a milestone.

It is also a release you cannot install blindly. Four things change on start; they are under [Before you upgrade](#before-you-upgrade).

## What is new

### Team workflow

- **Teams** have access to inboxes and take assignments. See [teams](/docs/headless/#teams).
- **Automatic assignment** hands a new conversation to an online member of the inbox or team, by round robin or by fewest open conversations, never past a cap you set per member. See [assignment](/docs/assignment/).
- **SLA policies** set first-reply, next-reply and resolution targets on business hours. The list shows the target, and a breach notifies the assignee. See [SLA](/docs/sla/).
- **Automation rules** label, assign, reply or call a webhook when a conversation matches, and a rule does not trigger on its own change. See [automation](/docs/automation/).
- **Macros** apply a reply, labels and a status in one step, with undo. **Saved views** are shared filters that show each member only what they may see. **Custom fields** can be required before a conversation closes. See [macros](/docs/headless/#macros), [saved views](/docs/headless/#saved-views) and [custom fields](/docs/custom-fields/).
- **Audit log**: owners read who changed access, credentials and data, with no message content or secret in it. See the [audit log](/docs/operations/#audit-log).
- **Trash and redaction**: deleting a conversation moves it to a 30-day trash from which it can be restored; a single message can be redacted and shows as removed, with its attachment. See [the Trash](/docs/headless/#deleting-conversations).

### E-mail

- **Cc and Bcc** on replies. People on Cc stay on the conversation, and their answers join it when the mail passed DMARC.
- **Forwarding a conversation** to an outside address brings the answer back into it, for your team only.
- **Proof of address**: an e-mail channel receives mail at its own address only after the workspace has proven it (see below).

Details are in [E-mail](/docs/email/).

### Reports

Reports counts conversations, replies, first-reply and resolution times, SLA results and ratings per day, week or month, by inbox, channel, label, team or member, with CSV export. An agent sees only their inboxes. The same figures are in the API and the MCP server. See [Reports](/docs/reports/).

### Integrations

- **Slack**: new conversations post to a channel and a reply in the thread becomes a note from the linked member. See [Slack](/docs/slack/).
- **GitHub and Linear**: link feedback to an issue and its state sets the feedback's progress. See [issue trackers](/docs/issue-trackers/).
- **Push sent by Yuva**: with APNs and FCM keys on an app channel, Yuva sends the reply push itself, without a webhook. See [mobile](/docs/mobile/#push-sent-by-yuva).
- Help centers, roadmap boards and What's new pages can answer on their own domain, and help articles import from Zendesk Guide and Help Scout. See [help centers](/docs/help-center/#import-from-zendesk-or-help-scout).

### Moving from another help desk

History imports now cover [Front](/docs/migrate-front/), [Freshdesk](/docs/migrate-freshdesk/), [Intercom](/docs/migrate-intercom/), [Crisp](/docs/migrate-crisp/) and [Chatwoot](/docs/migrate-chatwoot/), next to Zendesk, Help Scout and any mailbox over IMAP. Yuva only reads the old system. The history keeps its original dates, arrives read and sends no notification, webhook or push. See [Move to Yuva](/docs/migrate/).

### Sign-in and security

- **Single sign-on for members** with OpenID Connect or SAML 2.0, for the e-mail domains a workspace has claimed. Sign-in can create members on first use, and an owner can require it. See [single sign-on](/docs/sso/).
- Rate limits are counted in Postgres, so several server processes share them. Attachment limits are set per channel. The server exports OpenTelemetry traces when you point it at a collector.

## What 0.1 means

There is one version for the whole repository. The server, the API contract, the JavaScript, Swift, Kotlin and Go libraries, the Helm chart and the MCP manifests all report `0.1.0`. `/v1` and `/client/v1` in URLs are the version of the contract and stay as they are.

Yuva is still in beta. Until 1.0 the API and the database schema can change in a release. From here, upgrading follows the [operations guide](/docs/operations/#upgrades): read the release notes of every version in between, back up, change the image tag, start. Migrations run on start. Anything that needs more than that is written in the guide before the release, as it is for 0.1.0.

## Before you upgrade

[Upgrading to 0.1.0](/docs/operations/#upgrading-to-010) has the full steps. In short:

1. **Prove each e-mail address.** After the upgrade every e-mail channel is unproven. An unproven channel stops routing mail by its own address; mail to its forwarding address and sending keep working, and a catch-all receives nothing. Each workspace proves a channel with a DNS TXT record, with a code mailed to the address, or you vouch for it:

   ```sh
   docker compose exec yuva /yuva channel prove --all-unproven --workspace <ws>
   ```

   Without `--yes` the command only lists what it would prove. Run it again with `--yes --plan <code>`, using the code it printed.

2. **Redeploy the Email Worker.** `YUVA_INGRESS_ACCEPT_V2` is now off, so the server accepts only `v3` signatures. Redeploy the Worker in `edge/` before upgrading, or set `YUVA_INGRESS_ACCEPT_V2=true` until you have.

3. **Set `YUVA_INGRESS_AUTHSERV_ID`.** Greetings, rule replies and rating requests are now e-mailed only to senders whose mail passed DMARC. Without the setting none are sent, and the server warns at start.

4. **Know that a server without SMTP logs mail.** If `YUVA_SMTP_HOST` is not set, the server writes mail to its log, redacted, instead of dropping it. A sign-in code appears as one line with the address reduced to `*@domain`.

Two more changes to know. Background work now runs on more job queues, so a slow customer endpoint cannot delay assignment or notifications; nothing needs configuring. And single sign-on sessions are narrower than they were: a session made through a workspace's provider works only in that workspace, and only as a reader, so someone who signed in that way elsewhere signs in again with a code or passkey. Turning on "Require single sign-on" signs the covered members out of every session they did not make through your provider, in all their workspaces. Owners are exempt.

## Install

- **Hosted:** create an account on the [home page](/).
- **Self-hosted:** the [install guide](/docs/install/) starts Yuva and Postgres with Docker Compose in a few minutes; it also covers Helm for Kubernetes. The image is `ghcr.io/productdevbook/yuva:0.1.0`.

## What we have not tried

We tested the features above on a running server with recorded or fake providers: Mailpit for mail, Keycloak for single sign-on, fake Slack, GitHub, Linear, APNs and FCM endpoints, and recorded answers for the imports. Real DNS, real DMARC results at a mail provider, a real Slack app, pushes to real phones and imports from real help-desk accounts are not part of that. If you try them, tell us what breaks in an [issue](https://github.com/productdevbook/yuva/issues).
