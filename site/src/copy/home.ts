import type { I18n } from "@lingui/core"
import { msg } from "@lingui/core/macro"
import { docsRoot, links, VERSION, type Locale } from "@/lib/routes"
import type { IconName } from "@/lib/icons"

export type Item = { title: string; body: string; icon?: IconName }

export function homeCopy(i18n: I18n) {
  const version = VERSION
  const docs = (rel: string) => `${docsRoot(i18n.locale as Locale)}${rel}`
  const quickStart = docs("install/#quick-start-with-compose")
  const image = `ghcr.io/productdevbook/yuva:${VERSION}`

  return {
    meta: {
      title: i18n._(msg`Yuva: one open-source inbox for e-mail, live chat and in-app messages`),
      description: i18n._(
        msg`Yuva brings support e-mail, live chat and in-app conversations from all of your products into one shared inbox. Open source, self-hosted, one Go binary with Postgres.`
      ),
    },

    hero: {
      release: i18n._(msg`New in ${version}: bulk actions, contact merge and moving conversations`),
      title: i18n._(msg`One inbox for every product you run.`),
      lead: i18n._(
        msg`Yuva brings support e-mail, live chat and in-app conversations from all of your products into one shared inbox for your team. It is open source and runs as one Go binary with Postgres, so you can also host it yourself.`
      ),
      signUp: i18n._(msg`Create an account`),
      hosted: i18n._(msg`Hosted`),
      hostedBody: i18n._(msg`Create an account on our service and start without running a server.`),
      selfHosted: i18n._(msg`Self-hosted`),
      selfHostedBody: i18n._(msg`One Docker image and Postgres on your own server.`),
      quickStart: i18n._(msg`Read the quick start`),
      quickStartHref: quickStart,
      github: i18n._(msg`View on GitHub`),
      tags: i18n._(msg`Open source · Self-hosted · Pre-alpha`),
      tagsHosted: i18n._(msg`Open source · Hosted or self-hosted · Pre-alpha`),
      worksWith: i18n._(msg`Works with`),
      stack: ["Cloudflare Email Workers", "Amazon SES", "Postmark", "S3 · R2 · MinIO", "APNs · FCM", "Docker"],
      inbox: {
        title: i18n._(msg`One inbox`),
        rows: [
          {
            icon: "app",
            channel: i18n._(msg`In-app`),
            name: "Priya Raman",
            product: "Fieldnote",
            color: "bg-emerald-500",
            text: i18n._(msg`Notes stopped syncing after the update`),
            unread: true,
          },
          {
            icon: "chat",
            channel: i18n._(msg`Live chat`),
            name: "Lina Haddad",
            product: "Paperboat",
            color: "bg-sky-500",
            text: i18n._(msg`Can I schedule a newsletter in each subscriber's time zone?`),
            unread: true,
          },
          {
            icon: "mail",
            channel: i18n._(msg`E-mail`),
            name: i18n._(msg`Amara Okafor`),
            product: "Tally",
            color: "bg-indigo-500",
            text: i18n._(msg`Custom domain still shows as unverified`),
            unread: false,
          },
          {
            icon: "api",
            channel: i18n._(msg`API`),
            name: "Hiroshi Tanaka",
            product: "Tally",
            color: "bg-indigo-500",
            text: i18n._(msg`Invoice for September`),
            unread: false,
          },
        ] as { icon: "mail" | "chat" | "app" | "api"; channel: string; name: string; product: string; color: string; text: string; unread: boolean }[],
      },
      shotAlt: i18n._(
        msg`The Yuva panel: conversations from three products in one list, and an e-mail conversation open as a chat, with a teammate's note and a suggested reply.`
      ),
    },

    how: {
      kicker: i18n._(msg`How it works`),
      title: i18n._(msg`Messages come in on any channel. Your team answers in one place.`),
      steps: [
        {
          title: i18n._(msg`Connect your products`),
          body: i18n._(msg`Create one inbox per product and give it channels: a support address, a chat widget, an app key, an API.`),
        },
        {
          title: i18n._(msg`Answer from one panel`),
          body: i18n._(
            msg`Conversations from every channel share one list, one thread view and one set of tools. Members see only the inboxes they were given.`
          ),
        },
        {
          title: i18n._(msg`Let your backend react`),
          body: i18n._(
            msg`Signed webhooks tell your backend about new messages, so it can push a reply to a closed app with the keys it already has.`
          ),
        },
      ] as Item[],
    },

    channels: {
      kicker: i18n._(msg`Channels`),
      title: i18n._(msg`Every channel, one conversation model.`),
      lead: i18n._(
        msg`Live chat and asynchronous messaging differ only in settings. Whatever channel a conversation starts on, it looks and works the same in the panel, and each inbox chooses live or async.`
      ),
      email: {
        tag: i18n._(msg`E-mail`),
        title: i18n._(msg`E-mail done properly`),
        body: i18n._(
          msg`Point a support address at Yuva through a Cloudflare Email Worker, any MTA or a signed HTTP request. Replies go out through each channel's own SMTP account: SES, Postmark or your own relay.`
        ),
        points: [
          i18n._(msg`Threads follow In-Reply-To and References, and replies find their conversation even when a mail client drops the headers.`),
          i18n._(msg`Quotes and signatures are stripped from what you read; the full text, sanitized HTML and the original message are kept.`),
          i18n._(msg`Catch-all addresses per domain, and local+tag addresses that reach their channel.`),
        ],
        link: { href: docs("email/"), label: i18n._(msg`E-mail guide`) },
        letter: {
          from: i18n._(msg`Amara Okafor`),
          subject: i18n._(msg`Custom domain still shows as unverified`),
          body: i18n._(msg`I added the CNAME record two days ago, but the dashboard still says “unverified”. Is there anything else I need to do?`),
          quote: i18n._(msg`On Monday, Paperboat Support wrote: …`),
          marks: [i18n._(msg`quote hidden`), i18n._(msg`threaded`), "DMARC pass"],
        },
      },
      chat: {
        tag: i18n._(msg`Live chat`),
        title: i18n._(msg`Live chat and embedded threads`),
        body: i18n._(
          msg`One web component loaded by one script tag. Use it as a floating launcher on a website, or embed a conversation thread inside your own product's panel.`
        ),
        points: [
          i18n._(msg`Shadow DOM: your styles stay out, the widget's styles stay in.`),
          i18n._(msg`Presence, typing and read receipts in live inboxes; an expected reply time in async ones.`),
          i18n._(msg`When the visitor has left, unread replies follow them by e-mail, and their answer continues the same conversation.`),
        ],
        link: { href: docs("widget/"), label: i18n._(msg`Widget guide`) },
        shotAlt: i18n._(msg`The Yuva chat widget open on a newsletter tool's website: the visitor's question and a member's reply, with the member shown online.`),
      },
      app: {
        tag: i18n._(msg`In-app`),
        title: i18n._(msg`In-app messaging and feedback`),
        body: i18n._(
          msg`Native SDKs for iOS (YuvaKit, SwiftUI) and Android (Kotlin, Jetpack Compose) give your app a conversation list, threads with attachments and a feedback form, or a headless client for your own interface.`
        ),
        points: [
          i18n._(msg`Feedback with a category (bug, idea, praise, other), screenshots and device details: app version, build, system, device and screen.`),
          i18n._(msg`The panel filters feedback by category and counts what is still open.`),
          i18n._(msg`Users write as themselves through a short-lived token signed by your backend; Yuva never sees your user database.`),
        ],
        link: { href: docs("mobile/"), label: i18n._(msg`Mobile SDK guide`) },
        iosAlt: i18n._(msg`The messages screen of the iOS SDK in a sample app, with a feedback thread and a conversation.`),
      },
      api: {
        title: i18n._(msg`And an API for everything else`),
        body: i18n._(
          msg`A feedback form rendered by your own backend posts to /v1/feedback with a workspace API key. Scripts create inboxes, channels and keys, and host backends look up or delete contacts by their own user id.`
        ),
        link: { href: docs("api/"), label: i18n._(msg`API contract`) },
        tag: i18n._(msg`API`),
        code: `curl https://support.example.com/v1/feedback \\
  -H "Authorization: Bearer $YUVA_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "inbox_id": "8c5a…",
    "contact": { "external_id": "user_42" },
    "category": "idea",
    "body": "A dark mode for the reading view, please."
  }'`,
      },
    },

    panel: {
      kicker: i18n._(msg`The team panel`),
      title: i18n._(msg`Built for the people who answer.`),
      lead: i18n._(msg`The panel ships inside the binary. It is fast, made for the keyboard, and works on phones as an installable app.`),
      features: [
        { icon: "inbox", title: i18n._(msg`Shared inbox`), body: i18n._(msg`All, mine, unassigned, per inbox and per label; open, pending, snoozed and closed.`) },
        { icon: "assign", title: i18n._(msg`Assignment`), body: i18n._(msg`Assign conversations, set priority and hand over; every change is recorded in the thread.`) },
        { icon: "note", title: i18n._(msg`Internal notes`), body: i18n._(msg`Notes sit next to the replies and never reach the contact.`) },
        { icon: "tag", title: i18n._(msg`Labels and canned replies`), body: i18n._(msg`Label conversations and insert saved answers by typing / in the composer.`) },
        { icon: "bolt", title: i18n._(msg`Realtime`), body: i18n._(msg`New messages, typing and teammates' changes appear live over WebSocket.`) },
        { icon: "bell", title: i18n._(msg`Notifications`), body: i18n._(msg`Install the panel as an app and get Web Push on phones and desktops, with e-mail as a fallback.`) },
      ] as Item[],
      phoneAlt: i18n._(msg`The panel on a phone: the conversation list with each person's channel and latest message, and the tabs at the bottom.`),
    },

    developers: {
      kicker: i18n._(msg`For developers`),
      title: i18n._(msg`Made to be embedded in your products.`),
      lead: i18n._(msg`Yuva is built for teams that make their own software. Every surface has a typed contract and a small, documented integration.`),
      snippetsLabel: i18n._(msg`Integration examples`),
      snippets: [
        {
          id: "web",
          label: "Web",
          title: i18n._(msg`Website: the chat widget`),
          lang: "html",
          code: `<script src="https://support.example.com/yuva.js" defer></script>
<yuva-chat channel="yuva_pk_xxxxxxxxxxxxxxxx"></yuva-chat>`,
        },
        {
          id: "go",
          label: "Go",
          title: i18n._(msg`Your backend: an identity token in Go`),
          lang: "go",
          code: `import "github.com/productdevbook/yuva/sdk/go/identity"

token, err := identity.Sign(os.Getenv("YUVA_IDENTITY_SECRET"), identity.Claims{
	Subject:       user.ID,
	Email:         user.Email,
	EmailVerified: user.EmailConfirmed,
	Name:          user.Name,
	Attrs:         map[string]any{"plan": user.Plan},
})`,
        },
        {
          id: "swift",
          label: "iOS",
          title: i18n._(msg`iOS: YuvaKit and SwiftUI`),
          lang: "swift",
          code: `import YuvaKit

let yuva = YuvaClient(configuration: YuvaConfiguration(
    serverURL: URL(string: "https://support.example.com")!,
    channelKey: "yuva_pk_xxxxxxxxxxxxxxxx",
    identityToken: { try await MyAPI.yuvaIdentityToken() }
))

YuvaConversationsView(client: yuva, openConversationId: $openConversationId)`,
        },
        {
          id: "kotlin",
          label: "Android",
          title: i18n._(msg`Android: Kotlin and Compose`),
          lang: "kotlin",
          code: `val yuva = YuvaClient(
    context,
    YuvaConfiguration(
        serverUrl = "https://support.example.com",
        channelKey = "yuva_pk_xxxxxxxxxxxxxxxx",
        identityToken = { myApi.yuvaIdentityToken() },
    ),
)

YuvaConversationsView(client = yuva, openConversationId = conversationId, onClose = { finish() })`,
        },
        {
          id: "webhook",
          label: i18n._(msg`Webhooks`),
          title: i18n._(msg`Webhooks: verify the signature`),
          lang: "go",
          code: `import "github.com/productdevbook/yuva/sdk/go/webhook"

body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
if err := webhook.Verify(os.Getenv("YUVA_WEBHOOK_SECRET"), r.Header, body, 5*time.Minute); err != nil {
	http.Error(w, "bad signature", http.StatusUnauthorized)
	return
}`,
        },
      ],
      links: [
        { href: docs("widget/"), label: i18n._(msg`Web widget`) },
        { href: docs("identity/"), label: i18n._(msg`Identity tokens`) },
        { href: docs("mobile/"), label: i18n._(msg`Mobile SDKs`) },
        { href: docs("webhooks/"), label: i18n._(msg`Webhooks`) },
        { href: links.openapi, label: "OpenAPI" },
      ],
    },

    host: {
      kicker: i18n._(msg`Self-hosting`),
      title: i18n._(msg`Yours to run, on one small server.`),
      lead: i18n._(
        msg`Yuva is one Go binary and a Postgres database. The job queue, realtime events and search all live in Postgres, so there is nothing else to operate.`
      ),
      stack: [
        { k: "binary", v: "1" },
        { k: "postgres", v: "16+" },
        { k: "redis", v: "0" },
        { k: "workers", v: "0" },
      ],
      terminal: `docker pull ${image}
# compose.yaml and .env: copy both from the install guide
docker run --rm ${image} vapid-keys >> .env
docker compose up -d
docker compose exec yuva /yuva bootstrap --email you@example.com --workspace "Example" --name "Your Name"`,
      guide: i18n._(msg`Open the install guide`),
      guideHref: quickStart,
    },

    trust: {
      kicker: i18n._(msg`Security and privacy`),
      title: i18n._(msg`Careful with what it holds.`),
      lead: i18n._(
        msg`A support inbox holds other people's words. Yuva is built with that in mind, and says plainly that it has not had an independent security audit yet.`
      ),
      items: [
        { icon: "shield",
          title: i18n._(msg`Untrusted mail stays inert`),
          body: i18n._(msg`Incoming HTML is sanitized and shown in a sandboxed frame under a strict Content Security Policy; remote images wait until a member loads them.`),
        },
        { icon: "network",
          title: i18n._(msg`No requests into your network`),
          body: i18n._(msg`Webhook URLs, SMTP hosts and push endpoints are resolved and refused when they point at private, loopback or cloud metadata addresses.`),
        },
        { icon: "trash",
          title: i18n._(msg`Data you can delete`),
          body: i18n._(
            msg`Delete a contact, from the panel or by your own user id, and their conversations, messages and files go with them. Nothing goes to third parties except what a channel is set up to send.`
          ),
        },
      ] as Item[],
      policy: i18n._(msg`Security policy`),
    },

    open: {
      kicker: i18n._(msg`Open source and licensing`),
      title: i18n._(msg`Open source, and meant to stay that way.`),
      lead: i18n._(msg`Yuva uses a split license: the server stays open, and the pieces you embed in your own apps carry no copyleft obligations.`),
      licenses: [
        {
          tag: "AGPL-3.0",
          title: i18n._(msg`Server and panel`),
          body: i18n._(msg`Use it, change it, run it. If you run a modified Yuva as a network service for other people, you offer them its source.`),
        },
        {
          tag: "MIT",
          title: i18n._(msg`SDKs and API contract`),
          body: i18n._(
            msg`The widget, the Swift, Kotlin and Go SDKs and the OpenAPI contract are MIT, so they ship inside closed-source apps and store builds without obligations.`
          ),
        },
      ],
      read: i18n._(msg`Read LICENSING.md`),
    },

    faq: {
      kicker: i18n._(msg`FAQ`),
      title: i18n._(msg`Questions people ask.`),
      lead: i18n._(msg`Something missing? Open an issue on GitHub.`),
      items: [
        {
          q: i18n._(msg`Is Yuva ready for production?`),
          a: i18n._(
            msg`Not yet. It is pre-alpha: the API and the database schema can change without migration paths, and the code has not had an independent security audit. Try ${version} with test data and watch the repository for the next releases.`
          ),
        },
        {
          q: i18n._(msg`How is it different from hosted support tools?`),
          a: i18n._(
            msg`Yuva is built for one team that runs many products: each product is an inbox with its own branding, channels and members, all answered from one panel. You host it yourself, there is no per-seat pricing, and it stays small: one binary and Postgres.`
          ),
        },
        {
          q: i18n._(msg`What do I need to run it?`),
          a: i18n._(
            msg`A host with Docker, Postgres 16 or newer, an SMTP account for sign-in codes and notifications, and a host name with TLS behind a reverse proxy that passes WebSockets. Attachments go on a volume or into an S3-compatible bucket.`
          ),
        },
        {
          q: i18n._(msg`Can a closed-source app use the SDKs?`),
          a: i18n._(
            msg`Yes. The SDKs and the API contract are MIT. Only the server and the panel are AGPL, and that matters when you modify them and offer them to others as a network service.`
          ),
        },
        {
          q: i18n._(msg`Can I host it for my customers?`),
          a: i18n._(
            msg`Inside your own company, modified or not, yes. Offering a hosted service or a product under the Yuva name needs written permission; a renamed fork that keeps the AGPL is always allowed.`
          ),
        },
      ],
    },

    close: {
      title: i18n._(msg`Give every product's users a way to reach you.`),
      lead: i18n._(msg`Read the code, run ${version} on your machine, and follow along as it grows.`),
    },
  }
}

export type HomeCopy = ReturnType<typeof homeCopy>
