---
title: "Introducing Yuva: one inbox for every product you run"
description: "Why we built an open-source home for support e-mail, live chat and in-app conversations, what is in the first releases, and how to try it."
date: 2026-10-08
author: productdevbook
---

Small teams rarely run just one product. There is an app, its website, a side project and the tool that pays the bills, and each one collects messages from its users somewhere else. Support mail is forwarded into someone's personal mailbox, one site has a chat widget on a per-seat plan, and feedback from the mobile apps goes to a third-party SDK or nowhere at all.

Yuva is our answer to that: one shared inbox for every product you run. "Yuva" is Turkish for nest, or home, and that is the idea. Every conversation, whatever channel it started on, comes back to one place where the whole team can answer it.

## How it works

Every product gets an **inbox** with its own branding, language, business hours and members. You give each inbox the **channels** it needs:

- **E-mail.** Point a support address at Yuva through the included Cloudflare Email Worker, any MTA, or a signed HTTP request. Threads follow `In-Reply-To` and `References`, quotes and signatures are stripped from what you read, and replies go out through each channel's own SMTP account, such as Amazon SES or Postmark.
- **Live chat.** The `<yuva-chat>` web component loads with one script tag. Use it as a floating launcher on a website, or embed a conversation thread inside your own product. When a visitor has left, unread replies follow them by e-mail and their answer continues the same conversation.
- **In-app.** Native SDKs for iOS (YuvaKit and SwiftUI) and Android (Kotlin and Compose) give your app a conversation list, threads with attachments and a feedback form that carries the app version, device and system it came from.
- **API.** Everything else, from a feedback form rendered by your own backend to scripts that create inboxes and keys, goes through the REST API described in the OpenAPI contract.

Whatever the channel, a conversation looks and works the same in the **team panel**: one list, one thread view, assignment, internal notes, labels, canned replies, full-text search and realtime updates. The panel installs as an app and sends Web Push to phones and desktops.

Your own backend stays in the loop through **Standard Webhooks**. When a message arrives, Yuva tells your backend, and your backend can send the push notification to a closed app through APNs or FCM with the keys it already has.

## Small enough to run yourself

Yuva is one Go binary and a Postgres database. The job queue, realtime fan-out and search live in Postgres too, so there is no Redis and no separate worker to operate. The Docker image is built for `linux/amd64` and `linux/arm64`, and attachments go to a local volume or any S3-compatible storage.

If you would rather not run a server, you can also create an account on our hosted service from the home page.

## What is in the first releases

[Version 0.0.1](/releases/#v0.0.1), released on 7 October 2026, brought the whole foundation: workspaces with owners, admins and agents; sign-in with e-mailed codes and passkeys; inboxes, contacts and conversations; the team panel in English and Turkish; the e-mail channel with loop protection and bounce handling; the web widget; the mobile SDKs; feedback and webhooks; member notifications; and per-workspace retention.

[Version 0.0.2](/releases/#v0.0.2) followed a day later. Owners can delete a workspace and members can delete their own account, from the panel or through the API, and replies from an address a visitor typed but never confirmed are now marked as an unverified sender. The Android SDK is also available from JitPack.

## Open source, with a split license

The server and the panel are licensed under the AGPL-3.0, so improvements to them stay in the open. The SDKs and the API contract are MIT, so they can ship inside closed-source apps and store builds without obligations.

## Where it stands

Yuva is pre-alpha. The API and the database schema can still change without migration paths, and the code has not had an independent security audit. Try it with test data, read the code, and tell us what breaks.

- Read the [quick start](https://github.com/productdevbook/yuva/blob/main/docs/install.md#quick-start-with-compose) to run it with Docker Compose.
- Follow the [roadmap](https://github.com/productdevbook/yuva/blob/main/docs/roadmap.md) and the [releases](/releases/).
- Open an [issue](https://github.com/productdevbook/yuva/issues) with questions and ideas.

Thank you for reading, and welcome home.
