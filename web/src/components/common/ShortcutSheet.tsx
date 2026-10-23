import { Trans, useLingui } from "@lingui/react/macro"

import { Kbd } from "@/components/common/Kbd"
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog"

export const SHORTCUTS = {
  next: "j",
  reply: "r",
  note: "n",
  attach: "f",
  snooze: "s",
  hand: "a",
  labels: "l",
  progress: "o",
  close: "e",
  leave: "L",
  history: "h",
  contact: "c",
  more: ".",
  spam: "!",
  quoted: "q",
  discardSuggestion: "X",
  copyDetails: "y",
  search: "/",
  help: "?",
  previous: "k",
  home: "g h",
  contacts: "g c",
  conversations: "g i",
  team: "g t",
  reports: "g r",
  assistants: "g a",
  mentions: "g m",
  feedback: "g f",
  find: "F",
  pin: "p",
  unread: "u",
  docs: "g d",
  broadcasts: "g b",
  analytics: "g n",
  status: "g s",
  helpCenter: "g k",
  roadmap: "g p",
  updates: "g w",
  discussion: "P",
  incident: "i",
  article: "H",
  idea: "b",
  select: "x",
  linkSelected: "I",
  trash: "#",
  restore: "Z",
} as const

export const DISCUSSION_KEYS = {
  next: "j",
  previous: "k",
  reply: "r",
  replyTo: "R",
  answer: "a",
  approve: "y",
  hide: "x",
  spam: "!",
  title: "t",
  lock: "l",
  page: "o",
} as const

export const DOCS_KEYS = {
  pages: "1",
  discussions: "2",
  queue: "3",
  fromConversation: "c",
  install: "i",
  previousThread: "[",
  nextThread: "]",
} as const

export const BROADCAST_KEYS = {
  create: "c",
  design: "o",
  test: "t",
  send: "s",
  unschedule: "u",
  pause: "p",
  cancel: "X",
  report: "r",
  deliveries: "d",
  summary: "b",
  pace: "e",
} as const

export const LIST_KEYS = {
  create: "c",
  import: "i",
  lists: "1",
  archived: "2",
  subscribers: "1",
  importTab: "2",
  growth: "3",
  form: "4",
  settings: "5",
  add: "a",
  export: "E",
} as const

export const DESIGN_KEYS = {
  create: "c",
  mine: "1",
  gallery: "2",
  archived: "a",
  search: "/",
} as const

export const STATUS_KEYS = {
  overview: "1",
  components: "2",
  subscribers: "3",
  settings: "4",
  newPage: "c",
  addComponent: "n",
  addSubscriber: "a",
  preview: "v",
  openPage: "o",
  publishPage: "P",
} as const

export const INCIDENT_KEYS = {
  active: "1",
  scheduled: "2",
  past: "3",
  incident: "c",
  maintenance: "m",
  update: "r",
  publish: "p",
  resolve: "x",
  edit: "e",
  remove: "X",
  undoNotice: "u",
  link: "l",
  replyLinked: "R",
} as const

export const MONITOR_KEYS = {
  all: "1",
  down: "2",
  up: "3",
  paused: "4",
  add: "n",
  edit: "e",
  pause: "p",
  incident: "c",
  remove: "X",
} as const

export const HELP_KEYS = {
  articles: "1",
  collections: "2",
  report: "3",
  settings: "4",
  newArticle: "n",
  newCollection: "c",
  search: "f",
  openPage: "o",
  preview: "v",
  publishCenter: "P",
  previous: "[",
  next: "]",
  unpublish: "u",
  remove: "X",
  moveUp: "ArrowUp",
  moveDown: "ArrowDown",
} as const

export const ROADMAP_KEYS = {
  board: "1",
  settings: "2",
  newBoard: "c",
  nextColumn: "ArrowRight",
  previousColumn: "ArrowLeft",
  open: "Enter",
  move: "m",
  moveBack: "[",
  moveForward: "]",
  openPage: "o",
  publicBoard: "P",
  replyLinked: "A",
  linkFeedback: "f",
} as const

export const CHANGELOG_KEYS = {
  entries: "1",
  settings: "2",
  create: "c",
  manage: "m",
  newEntry: "n",
  openPage: "o",
  publish: "P",
  preview: "v",
} as const

export const ENTRY_KEYS = {
  schedule: "s",
  unpublish: "u",
  mail: "m",
  remove: "X",
  previous: "[",
  next: "]",
} as const

export const DESIGNER_KEYS = {
  edit: "1",
  desktop: "2",
  phone: "3",
  dark: "4",
  add: "a",
  theme: "t",
  image: "i",
  versions: "v",
  checks: "c",
} as const

export function keyLabel(k: string): string[] {
  if (k.includes(" ")) return k.split(" ").flatMap(keyLabel)
  return /^[A-Z]$/.test(k) ? ["⇧", k] : [k.toUpperCase()]
}

const apple = typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.platform)

export const mod = apple ? "⌘" : "Ctrl"

export const alt = apple ? "⌥" : "Alt"

export function ShortcutSheet({
  open,
  onOpenChange,
  home,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  home?: boolean
}) {
  const { t } = useLingui()
  const k = keyLabel
  const groups: { title: string; items: [string[], string][] }[] = [
    {
      title: t`Answering`,
      items: [
        [k(SHORTCUTS.reply), t`Write a reply`],
        [k(SHORTCUTS.note), t`Write a note for the team`],
        [[mod, "↵"], t`Send and close`],
        [["⇧", mod, "↵"], t`Send only`],
        [["↵"], t`Send or add the note, in a chat or app conversation (⇧↵ for a new line)`],
        [["Tab"], t`Use the suggested reply`],
        [k(SHORTCUTS.discardSuggestion), t`Discard the suggested reply`],
        [k(SHORTCUTS.attach), t`Attach files`],
        [["/"], t`Canned replies, in the reply box`],
        [k(SHORTCUTS.leave), t`Leave it to the teammate who is typing`],
        [["Del"], t`Remove the message or note you clicked on`],
      ],
    },
    {
      title: t`This conversation`,
      items: [
        [k(SHORTCUTS.hand), t`Assign or hand to a teammate`],
        [k(SHORTCUTS.snooze), t`Snooze`],
        [k(SHORTCUTS.labels), t`Labels`],
        [k(SHORTCUTS.progress), t`Set the feedback progress`],
        [k(SHORTCUTS.close), t`Close or reopen`],
        [k(SHORTCUTS.contact), t`Contact details`],
        [k(SHORTCUTS.history), t`Other conversations`],
        [k(SHORTCUTS.more), t`Priority, inbox, spam and more`],
        [k(SHORTCUTS.spam), t`Mark or unmark as spam`],
        [k(SHORTCUTS.trash), t`Move to the Trash`],
        [k(SHORTCUTS.restore), t`Restore from the Trash`],
        [k(SHORTCUTS.quoted), t`Show or hide quoted text`],
        [k(SHORTCUTS.copyDetails), t`Copy the feedback details`],
        [k(SHORTCUTS.find), t`Search in this conversation`],
        [k(SHORTCUTS.pin), t`Pin to the top of your list, or unpin`],
        [k(SHORTCUTS.unread), t`Mark as unread and go back to the list`],
        [k(SHORTCUTS.discussion), t`Turn into a public discussion`],
        [k(SHORTCUTS.incident), t`Link to a status incident`],
        [k(SHORTCUTS.article), t`Send a help article`],
        [k(SHORTCUTS.idea), t`Link to a roadmap idea`],
      ],
    },
    {
      title: t`Conversation list`,
      items: [
        [["↑", "↓"], t`Move through the list`],
        [["↵"], t`Open the conversation`],
        [[k(SHORTCUTS.next)[0], k(SHORTCUTS.previous)[0]], t`Open the next or previous conversation`],
        [[SHORTCUTS.search], t`Search conversations`],
        [k(SHORTCUTS.select), t`Select the conversation under the cursor`],
        [k(SHORTCUTS.linkSelected), t`Link the selected conversations to an incident`],
        [k(SHORTCUTS.trash), t`Move the selected conversations to the Trash`],
        [k(SHORTCUTS.restore), t`Restore the selected conversations, in the Trash`],
        [["Esc"], t`Clear the selection`],
      ],
    },
    {
      title: t`Discussion`,
      items: [
        [["↑", "↓"], t`Move through the posts`],
        [k(DISCUSSION_KEYS.reply), t`Reply to the discussion`],
        [k(DISCUSSION_KEYS.replyTo), t`Reply to the selected post`],
        [k(DISCUSSION_KEYS.answer), t`Mark the selected reply as the answer, or clear it`],
        [k(DISCUSSION_KEYS.approve), t`Approve or show the selected post`],
        [k(DISCUSSION_KEYS.hide), t`Hide the selected post`],
        [k(DISCUSSION_KEYS.spam), t`Mark the selected post as spam`],
        [k(DISCUSSION_KEYS.title), t`Edit the title`],
        [k(DISCUSSION_KEYS.lock), t`Lock or unlock`],
        [k(DISCUSSION_KEYS.page), t`Open on its page`],
        [[mod, "↵"], t`Send the reply`],
      ],
    },
    {
      title: t`Discussions and approvals`,
      items: [
        [["↑", "↓"], t`Move through the list`],
        [["↵"], t`Open the discussion`],
        [k(DISCUSSION_KEYS.approve), t`Approve the selected post`],
        [k(DISCUSSION_KEYS.hide), t`Hide the selected post`],
        [k(DISCUSSION_KEYS.spam), t`Mark the selected post as spam`],
      ],
    },
    {
      title: t`Docs`,
      items: [
        [k(DOCS_KEYS.pages), t`Pages`],
        [k(DOCS_KEYS.discussions), t`Discussions`],
        [k(DOCS_KEYS.queue), t`Waiting for approval`],
        [k(DOCS_KEYS.fromConversation), t`From a conversation`],
        [k(DOCS_KEYS.install), t`How to add`],
        [[DOCS_KEYS.previousThread, DOCS_KEYS.nextThread], t`Previous or next discussion`],
        [["Esc"], t`Close the discussion`],
      ],
    },
    {
      title: t`Broadcasts`,
      items: [
        [["↑", "↓"], t`Move through the broadcasts`],
        [["↵"], t`Open the broadcast`],
        [k(BROADCAST_KEYS.create), t`New broadcast`],
        [k(BROADCAST_KEYS.design), t`Edit the design`],
        [k(BROADCAST_KEYS.test), t`Send a test`],
        [k(BROADCAST_KEYS.send), t`Review, then send or schedule`],
        [k(BROADCAST_KEYS.unschedule), t`Back to draft`],
        [k(BROADCAST_KEYS.pause), t`Pause or resume sending`],
        [k(BROADCAST_KEYS.cancel), t`Cancel the broadcast`],
        [k(BROADCAST_KEYS.report), t`Report`],
        [k(BROADCAST_KEYS.deliveries), t`Recipients`],
        [k(BROADCAST_KEYS.summary), t`Broadcast`],
        [k(BROADCAST_KEYS.pace), t`Change pace`],
      ],
    },
    {
      title: t`Lists`,
      items: [
        [["↑", "↓"], t`Move through the lists`],
        [["↵"], t`Open the list`],
        [k(LIST_KEYS.create), t`New list`],
        [k(LIST_KEYS.import), t`Import CSV`],
        [k(LIST_KEYS.lists), t`Lists`],
        [k(LIST_KEYS.archived), t`Archived`],
        [k(LIST_KEYS.subscribers), t`Subscribers`],
        [k(LIST_KEYS.importTab), t`Import`],
        [k(LIST_KEYS.growth), t`Growth`],
        [k(LIST_KEYS.form), t`Form`],
        [k(LIST_KEYS.settings), t`Settings`],
        [k(LIST_KEYS.add), t`Add subscriber`],
        [k(LIST_KEYS.export), t`Export`],
      ],
    },
    {
      title: t`Designs`,
      items: [
        [["↑", "↓"], t`Move through the designs`],
        [["↵"], t`Open the design`],
        [k(DESIGN_KEYS.create), t`New design`],
        [k(DESIGN_KEYS.mine), t`My designs`],
        [k(DESIGN_KEYS.gallery), t`Gallery`],
        [k(DESIGN_KEYS.search), t`Search designs`],
        [k(DESIGN_KEYS.archived), t`Show archived designs`],
      ],
    },
    {
      title: t`Status pages`,
      items: [
        [["↑", "↓"], t`Move through the status pages`],
        [k(STATUS_KEYS.overview), t`Overview`],
        [k(STATUS_KEYS.components), t`Components`],
        [k(STATUS_KEYS.subscribers), t`Subscribers`],
        [k(STATUS_KEYS.settings), t`Page settings`],
        [k(STATUS_KEYS.newPage), t`New status page`],
        [k(STATUS_KEYS.addComponent), t`Add a component`],
        [k(STATUS_KEYS.addSubscriber), t`Add a subscriber`],
        [k(STATUS_KEYS.preview), t`Preview the page`],
        [k(STATUS_KEYS.openPage), t`Open the public page`],
        [k(STATUS_KEYS.publishPage), t`Publish or unpublish the page`],
      ],
    },
    {
      title: t`Incidents`,
      items: [
        [["↑", "↓"], t`Move through the incidents`],
        [k(INCIDENT_KEYS.active), t`Active`],
        [k(INCIDENT_KEYS.scheduled), t`Scheduled`],
        [k(INCIDENT_KEYS.past), t`Past`],
        [k(INCIDENT_KEYS.incident), t`New incident`],
        [k(INCIDENT_KEYS.maintenance), t`Plan maintenance`],
        [k(INCIDENT_KEYS.update), t`Post an update`],
        [k(INCIDENT_KEYS.publish), t`Publish the draft`],
        [k(INCIDENT_KEYS.resolve), t`Resolve or complete`],
        [k(INCIDENT_KEYS.edit), t`Edit the incident`],
        [k(INCIDENT_KEYS.remove), t`Delete the draft`],
        [k(INCIDENT_KEYS.undoNotice), t`Undo the waiting notice`],
        [k(INCIDENT_KEYS.link), t`Link conversations`],
        [k(INCIDENT_KEYS.replyLinked), t`Reply to linked conversations`],
      ],
    },
    {
      title: t`Monitors`,
      items: [
        [["↑", "↓"], t`Move through the monitors`],
        [k(MONITOR_KEYS.all), t`All monitors`],
        [k(MONITOR_KEYS.down), t`Down`],
        [k(MONITOR_KEYS.up), t`Up`],
        [k(MONITOR_KEYS.paused), t`Paused`],
        [k(MONITOR_KEYS.add), t`Add a monitor`],
        [k(MONITOR_KEYS.edit), t`Edit the monitor`],
        [k(MONITOR_KEYS.pause), t`Pause or resume the monitor`],
        [k(MONITOR_KEYS.incident), t`Open an incident`],
        [k(MONITOR_KEYS.remove), t`Delete the monitor`],
      ],
    },
    {
      title: t`Help center`,
      items: [
        [["↑", "↓"], t`Move through the articles or collections`],
        [["↵"], t`Open the article or collection`],
        [k(HELP_KEYS.articles), t`Articles`],
        [k(HELP_KEYS.collections), t`Collections`],
        [k(HELP_KEYS.report), t`Help report`],
        [k(HELP_KEYS.settings), t`Help center settings`],
        [k(HELP_KEYS.newArticle), t`New article`],
        [k(HELP_KEYS.newCollection), t`New collection`],
        [k(HELP_KEYS.search), t`Search the articles`],
        [k(HELP_KEYS.preview), t`Preview the help center`],
        [k(HELP_KEYS.openPage), t`Open the public help center`],
        [k(HELP_KEYS.publishCenter), t`Publish or unpublish the help center`],
        [[alt, "↑", "↓"], t`Move the selected collection`],
      ],
    },
    {
      title: t`Help article`,
      items: [
        [[mod, "S"], t`Save the draft now`],
        [[mod, "↵"], t`Publish, or submit the draft`],
        [k(HELP_KEYS.unpublish), t`Unpublish the translation`],
        [k(HELP_KEYS.remove), t`Delete the article`],
        [[HELP_KEYS.previous, HELP_KEYS.next], t`Previous or next language`],
        [[mod, "B"], t`Bold`],
        [[mod, "I"], t`Italic`],
        [[alt, "K"], t`Add a link`],
        [[alt, "I"], t`Add an image`],
        [[alt, "P"], t`Show or hide the preview`],
        [[alt, "C"], t`Show or hide the original text beside a translation`],
        [[alt, "H"], t`Revisions`],
      ],
    },
    {
      title: t`Roadmap`,
      items: [
        [["↑", "↓", "←", "→"], t`Move through the ideas`],
        [["↵"], t`Open the idea`],
        [k(ROADMAP_KEYS.board), t`Board`],
        [k(ROADMAP_KEYS.settings), t`Board settings`],
        [k(ROADMAP_KEYS.newBoard), t`Create the board`],
        [k(ROADMAP_KEYS.move), t`Move to a stage`],
        [[ROADMAP_KEYS.moveBack, ROADMAP_KEYS.moveForward], t`Move to the previous or next stage`],
        [k(ROADMAP_KEYS.openPage), t`Open the public board`],
        [k(ROADMAP_KEYS.publicBoard), t`Make the board public or private`],
        [k(ROADMAP_KEYS.linkFeedback), t`Link feedback to the idea`],
        [k(ROADMAP_KEYS.replyLinked), t`Reply to linked feedback`],
      ],
    },
    {
      title: t`What’s new`,
      items: [
        [["↑", "↓"], t`Move through the entries`],
        [["↵"], t`Open the entry`],
        [k(CHANGELOG_KEYS.entries), t`Entries`],
        [k(CHANGELOG_KEYS.settings), t`Changelog settings`],
        [k(CHANGELOG_KEYS.create), t`Create the changelog`],
        [k(CHANGELOG_KEYS.manage), t`Manage the changelogs`],
        [k(CHANGELOG_KEYS.newEntry), t`New entry`],
        [k(CHANGELOG_KEYS.openPage), t`Open the public changelog`],
        [k(CHANGELOG_KEYS.preview), t`Preview`],
        [k(CHANGELOG_KEYS.publish), t`Publish or unpublish the changelog`],
      ],
    },
    {
      title: t`What’s new entry`,
      items: [
        [[mod, "S"], t`Save the draft now`],
        [[mod, "↵"], t`Publish, or submit the draft`],
        [k(ENTRY_KEYS.schedule), t`Schedule the entry`],
        [k(ENTRY_KEYS.unpublish), t`Unpublish or unschedule the entry`],
        [k(ENTRY_KEYS.mail), t`Mail the entry to subscribers`],
        [k(ENTRY_KEYS.remove), t`Delete the entry`],
        [[ENTRY_KEYS.previous, ENTRY_KEYS.next], t`Previous or next language`],
        [[mod, "B"], t`Bold`],
        [[mod, "I"], t`Italic`],
        [[alt, "K"], t`Add a link`],
        [[alt, "I"], t`Add an image`],
        [[alt, "P"], t`Show or hide the preview`],
        [[alt, "C"], t`Show or hide the original text beside a translation`],
      ],
    },
    {
      title: t`E-mail designer`,
      items: [
        [[mod, "Z"], t`Undo`],
        [["⇧", mod, "Z"], t`Redo`],
        [[mod, "S"], t`Save now and keep a version`],
        [["↑", "↓"], t`Select the previous or next block`],
        [[alt, "↑", "↓"], t`Move the selected block`],
        [[mod, "D"], t`Duplicate the selected block`],
        [["⌫"], t`Delete the selected block`],
        [["Esc"], t`Leave the text, then the block`],
        [k(DESIGNER_KEYS.add), t`Add a block`],
        [k(DESIGNER_KEYS.theme), t`Colours, fonts and width`],
        [k(DESIGNER_KEYS.image), t`Choose the selected block’s image`],
        [k(DESIGNER_KEYS.versions), t`Versions`],
        [k(DESIGNER_KEYS.checks), t`Checks`],
        [
          [DESIGNER_KEYS.edit, DESIGNER_KEYS.desktop, DESIGNER_KEYS.phone, DESIGNER_KEYS.dark],
          t`Edit, or preview on a desktop, a phone or in dark mode`,
        ],
      ],
    },
    {
      title: t`Everywhere`,
      items: [
        ...(home ? [[k(SHORTCUTS.home), t`Home`] as [string[], string]] : []),
        [k(SHORTCUTS.conversations), t`Conversations`],
        [k(SHORTCUTS.mentions), t`Mentions`],
        [k(SHORTCUTS.feedback), t`Feedback`],
        [k(SHORTCUTS.contacts), t`Contacts`],
        [k(SHORTCUTS.team), t`Team`],
        [k(SHORTCUTS.reports), t`Reports`],
        [k(SHORTCUTS.docs), t`Docs`],
        [k(SHORTCUTS.broadcasts), t`Broadcasts`],
        [k(SHORTCUTS.status), t`Status`],
        [k(SHORTCUTS.helpCenter), t`Help`],
        [k(SHORTCUTS.roadmap), t`Roadmap`],
        [k(SHORTCUTS.updates), t`What’s new`],
        [k(SHORTCUTS.analytics), t`Analytics`],
        [k(SHORTCUTS.assistants), t`Assistants`],
        [[mod, "K"], t`Everything`],
        [[mod, ","], t`Settings`],
        [[mod, "\\"], t`Collapse or expand the sidebar`],
        [[SHORTCUTS.help], t`This list`],
        [["Esc"], t`Close a panel or leave a field`],
      ],
    },
  ]
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85svh] overflow-y-auto sm:max-w-2xl" data-testid="shortcut-sheet">
        <DialogHeader>
          <DialogTitle>
            <Trans>Keyboard shortcuts</Trans>
          </DialogTitle>
        </DialogHeader>
        <div className="grid gap-x-8 gap-y-6 sm:grid-cols-2">
          {groups.map((g) => (
            <section key={g.title} className="flex flex-col">
              <h3 className="mb-1 text-small font-medium text-faint">{g.title}</h3>
              <ul className="flex flex-col">
                {g.items.map(([ks, label]) => (
                  <li key={label} className="flex items-center justify-between gap-3 border-t py-1.5 text-body">
                    <span>{label}</span>
                    <span className="flex shrink-0 gap-1">
                      {ks.map((key) => (
                        <Kbd key={key}>{key}</Kbd>
                      ))}
                    </span>
                  </li>
                ))}
              </ul>
            </section>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  )
}
