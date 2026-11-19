# Panel redesign (M41)

Status: planned, milestone M41 (M41.1–M41.7). Builder: yuva-panel (`web/`); yuva-api only where a
step names an endpoint that does not exist yet. Built in a fresh session per step, from this file.
The approved design is a set of interactive page drawings kept outside the repository; every
decision it encodes is written down here, so this file alone is enough to build from.

The panel today is a messaging-app shell: an icon rail, a list column and a main area. It fits
conversations and fits nothing else; Broadcasts, Analytics, Status, Help center and Settings are
squeezed into the list column, Home is a setup page, and a section has no place for its own views.
The redesign keeps every URL, endpoint, feature and shortcut and changes only how pages are laid
out and reached.

## 1. Decisions

1. **shadcn for everything.** Every control comes from `web/src/components/ui` (the shadcn
   `base-vega` set already installed). A missing one is added with `npx shadcn add`, never written
   by hand. No new hand-rolled buttons, inputs, menus, tables, switches, toasts or dialogs; the
   existing hand-rolled ones on a page are replaced when that page's step runs. Mapping:

   | Need | shadcn component |
   |---|---|
   | Header menus (workspace and product, pages, account, bell) | `DropdownMenu`, `Popover` |
   | Header search | `Command` (the existing palette) |
   | Section sidebar | `Sidebar` (`SidebarGroup`, `SidebarMenuButton` with `isActive`) |
   | Lists of records with columns | `Table` |
   | Lists of messages, people, activity | `Item` |
   | Bordered blocks (Home boxes, settings groups) | `Card` |
   | Forms | `Field`, `Input`, `Textarea`, `NativeSelect` or `Select`, `RadioGroup`, `Checkbox`, `Switch` |
   | Segmented filters (All, Unread; periods) | `ToggleGroup` |
   | Buttons and button rows | `Button`, `ButtonGroup` |
   | Bulk selection bar | `Checkbox` + `ButtonGroup` |
   | Toasts with undo | `sonner` (`toast` with an `action`) |
   | Empty states | `Empty` |
   | Progress (setup card, imports) | `Progress` |
   | Statuses and tags | `Badge` |
   | Confirmations that cannot be undone | `AlertDialog` |

2. **Header.** One bar, 40px high, not sticky, on every page except setup flows and the OAuth
   consent page. Left: the Yuva mark, the workspace and product selector (moved from the top of
   the sidebar), `/`, the current page's name as a menu of all pages. Right: search (opens the
   palette, `⌘K`), the bell with unread notifications, the avatar menu (availability switch,
   theme, Getting started, account, sign out). The pages menu has three groups:
   - Work: Home, Inbox, Mail, People.
   - Reach: Broadcasts, Analytics, Feedback and roadmap, Help center, Status.
   - Workspace: Reports, Team, Settings, Imports.

   Entries follow the inbox filter and permissions exactly as the rail does now.
3. **Section sidebar.** Inside a section, a left `Sidebar` (232px) lists the section's places:
   views and sub-pages, grouped under small headings, each with a plain count on the right.
   It is single-select navigation:
   - Exactly one item is highlighted (`aria-current="page"`).
   - Filters and scopes (labels, channel, site, period, plan) never go in it; they go in a filter
     row above the list or in the page header.
   - Home has no section sidebar.
   - On a phone the sidebar becomes a horizontal scrolling strip under the header.
4. **Page body.** The page has these parts, top to bottom:
   - Title (22px) with a short muted subtitle, and the primary action on the right.
   - Then the filter row.
   - Then one bordered `Card` or `Table` holding the list.

   Type is normal: 13–14px body, 15px section headings, no hero blocks. A detail opens in the same
   page with a small back link, at its own URL.
5. **Forms are plain.** Adding a thing opens a plain inline form on the page: label above a 36px
   input, muted help text below, the primary and the secondary button at the bottom. It is not a
   modal wizard. Multi-step work (channel setup, imports, new broadcast) shows a "1. A · 2. B · 3. C"
   step line and one short form per step. `Dialog` is only for confirmations and quick edits of one
   field.
6. **Lists act in bulk.** Every list whose rows can be archived, closed, moved or deleted has row
   checkboxes, a select-all checkbox, and a bar of the actions for the selection. Delete is red;
   it either moves rows to Trash or offers an undo toast for a few seconds.
7. **Home is an overview, not setup.** See §3. Setup lives in a card on Home and at Getting started.
8. **URLs stay.** Every current route keeps working. A section sidebar item is a route; an item
   that is now a filter (a label in Inbox) becomes a query parameter on the section's route. The
   rail's `G` shortcuts keep their targets.

## 2. Pages

Each page lists its sidebar, then the main area. "Exists" means the feature is already built and
only moves.

- **Inbox** (`/conversations`):
  - Sidebar views (counts over all inboxes):
    - Mine, Unassigned, All open, Late, Snoozed, Done
    - Mentions, Assistants (both exist)
  - Sidebar inboxes: one per inbox. Clicking an inbox shows that inbox's open conversations.
  - Filter row: label, channel, site (from the existing Filter popover). Search stays at the top.
  - Bulk bar: assign to me, snooze, close.
  - The conversation opens in the main area as now.
- **Mail** (`/mail`):
  - Sidebar mailboxes: one per mailbox; the chosen one is shown in the page subtitle.
  - Sidebar folders: Inbox, Starred, Snoozed, Sent, Drafts, Scheduled, Archive, Trash, Filters.
    Only the folder is highlighted.
  - Rows show sender, subject with a preview, and time. It is a list, not a table.
  - Bulk bar:
    - Archive, Mark as read, Mark as unread, Star
    - Delete (to Trash), with undo
- **People** (`/contacts`):
  - Sidebar: All people, Customers, Visitors, Unsubscribed, Companies.
  - `Table` columns: person, company, plan, labels, last seen, conversations.
  - Filter row: label, plan.
  - Contact detail: conversations, internal notes, labels, mailing lists, merge.
  - Bulk delete (exists).
- **Team** (`/team`):
  - Sidebar: Members, Teams, Invites.
  - Member detail as now. The invite form is inline.
- **Reports** (`/reports`):
  - Sidebar: Today, Trends, Ratings, Members.
  - Filter row: period, inbox, channel, team (`ToggleGroup`).
  - Each figure shows its change against the period before.
- **Broadcasts** (`/broadcasts`):
  - Sidebar group Broadcasts: All, Drafts and approvals, Scheduled, Sent, Triggered.
  - Sidebar group Audience: Subscribers.
  - Sidebar group Content: Designs.
  - The sender line ("Sending from … · SPF, DKIM, DMARC ok") sits under the title.
  - Lists (`/broadcasts/lists`) is Subscribers; Triggered and Designs keep their routes.
- **Analytics** (`/analytics`):
  - Sidebar Reports: Overview, Realtime, Sources, Pages, Locations, Devices, Goals, Events,
    Funnels, Support.
  - Sidebar group This site: Settings.
  - The property picker is in the page header, not the sidebar.
  - Analytics settings: Install, alerts, sharing, exports. Adding a property is a plain form; after
    it, the Install tab opens with the platform chosen.
- **Feedback and roadmap** (`/board`, `/feedback`):
  - Sidebar: Feedback, Roadmap, Bugs.
  - Roadmap columns: Ideas, Planned, In progress, Done, with votes.
  - Idea detail: stage, linked feedback, visibility.
- **Help center** (`/knowledge`, `/updates`):
  - Sidebar: Articles, Collections, Discussions, Search gaps, Updates (What's new).
  - Article editor and discussion threads as now.
- **Status** (`/status`):
  - Sidebar: Pages, Incidents, Monitors.
  - Incident detail: timeline and an "Post an update" form beside it.
- **Settings** (`/settings`). The sidebar groups replace today's settings column, at the same
  URLs:
  - Workspace: General, Products.
  - Channels: Inboxes and channels, Addresses.
  - Conversations: Labels, Saved replies, Macros, Automation, Reply times, Fields, Signature.
  - Developer: API keys, Webhooks, Connected apps, Issue trackers, Slack.
  - Security: SSO, Audit log.
  - You: Profile, Notifications, Appearance.

  A group of settings is a `Card` with rows (label and help on the left, control on the right) and
  a Save button.
- **Imports** (`/settings/migrations`, `/migrations/*`):
  - Sidebar: Imports, New import.
  - List with progress and status; the detail page has pause, resume, cancel and finish.
  - New import: "1. Source · 2. Connect · 3. Choose · 4. Review". The read-only notice is shown on
    every step.
- **Setup flows** (`/setup/*`). There is no header and no sidebar.
  - Each flow is "1. Address · 2. Replies · 3. Receiving · 4. Test" style steps, one plain form per
    step, with Back, Cancel and Continue.
  - Finishing or cancelling returns to where the flow was opened from: Home or Getting started.

## 3. Home (`/home`)

Top row:
- "Good morning, Ada" and the date with the workspace name.
- On the right:
  - The availability switch with "N of M online".
  - The Getting started button, only while the setup card is hidden.

Then, top to bottom, two columns on a desktop and one on a phone:

1. **Setup card** (owners and admins, until every setup item is done or the card is hidden):
   - Title "Set up \<workspace\>" or "Finish setting up \<workspace\>", with "N of 4 done" and a
     `Progress`.
   - Only the items not done yet, each with its step count and a Set up button.
   - "All steps" opens Getting started; "Hide" hides the card for this member.
   - When nothing is set up, the card and an empty Waiting for you are the whole page.
2. **My conversations** (left): the member's open assigned conversations, most urgent first.
   - Each row shows what is waiting: "Reply in 4m" (red within the SLA's last 10 minutes), waiting
     on the customer, or "Back 14:00" when snoozed.
   - "Open in Inbox" goes to the Mine view.
3. **Waiting for you** (left): one row per thing that needs someone, each linking to its page.
   - Late conversations.
   - Unassigned conversations (one row with the count per inbox).
   - Broadcasts waiting for this member's approval.
   - Open incidents.
   - Discussions needing a reply.
   - Imports needing attention.

   Agents do not see approvals or imports.
4. **Today** (left): open conversations, opened · closed, median first reply, visitors now,
   rating, the last broadcast's opens. Each figure links to its report. Agents see their own open
   conversations, first reply and rating.
5. **Later today** (right): snoozed conversations coming back, scheduled broadcasts, imports
   expected to finish, in time order. Agents see only conversations.
6. **Products** (right, owners and admins): each product with its open conversations and its
   status page state.
7. **Recent** (right): the team's last actions (closed, rated, posted an update, published, sent).

Data comes from existing endpoints:
- `/v1/conversations/counts` and `/v1/conversations` (assignee, status, snooze).
- `/v1/stats`, `/v1/broadcasts`, `/v1/status-pages/{pageId}/incidents`, `/v1/discussions`,
  `/v1/migrations`, `/v1/members`.

Before M41.2 starts, yuva-api checks each block against `openapi/openapi.yaml`. A block with no
source (likely Recent, and Later today's import estimate) gets one small read endpoint in M41.2,
contract first. Otherwise the block is left out and named in the step's report.

## 4. Steps

Each step is one session and one commit (or a few). Each step ends with:
- the `web/` check from CLAUDE.md;
- English, Turkish and German strings through Lingui;
- device-tester screenshots at 1440 and 390 wide, in light and dark, of every page the step
  touched.

- **M41.1 Shell.**
  - Header and section sidebar layout replace the rail and the list column, using the `Sidebar`
    component.
  - Every section gets its sidebar items; the content stays as it is.
  - The phone strip.
  - `architecture.md` › Panel: rewrite the main-screen description.
- **M41.2 Home.** §3, with the setup card and Getting started. Setup flows return to Home.
- **M41.3 Inbox and Mail.** Inbox views and inboxes in the sidebar, labels in the filter row, the
  Mail folders and the bulk bar.
- **M41.4 People, Team, Reports.**
- **M41.5 Broadcasts and Analytics.** The Broadcasts sidebar; Analytics settings forms.
- **M41.6 Feedback and roadmap, Help center, Status.**
- **M41.7 Settings, Imports, Setup flows.** Setup flows become one-form-per-step.

Not part of it: new features, API changes beyond §3's read endpoints, the public pages (status,
help, roadmap sites), the widget and the SDKs.

Accept: every route that worked before M41 still opens. Each section shows its sidebar with
exactly one highlighted item. On a 390px phone no page scrolls sideways. Light and dark both pass.
An agent's Home shows their own conversations and no approvals. An owner finishes mail setup from
the Home card and lands back on Home with the card at 1 of 4.
