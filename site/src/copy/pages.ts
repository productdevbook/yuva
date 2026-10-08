import type { I18n } from "@lingui/core"
import { msg } from "@lingui/core/macro"

export function blogCopy(i18n: I18n) {
  return {
    title: i18n._(msg`Blog`),
    metaTitle: i18n._(msg`Yuva blog`),
    description: i18n._(msg`News, release stories and notes from building Yuva, the open-source inbox for every product you run.`),
    lead: i18n._(msg`News, release stories and notes from building Yuva.`),
    english: i18n._(msg`Posts are published in English.`),
    rss: i18n._(msg`RSS feed`),
    read: i18n._(msg`Read the post`),
    back: i18n._(msg`All posts`),
    by: i18n._(msg`By`),
    quickStart: i18n._(msg`Read the quick start`),
    signUp: i18n._(msg`Create an account`),
    kicker: i18n._(msg`From the team`),
    latest: i18n._(msg`Latest post`),
    more: i18n._(msg`More posts`),
    follow: i18n._(msg`Follow the blog`),
    followBody: i18n._(msg`New posts appear in the RSS feed as soon as they are published.`),
    onThisPage: i18n._(msg`On this page`),
    share: i18n._(msg`Share`),
    copyLink: i18n._(msg`Copy link`),
    copied: i18n._(msg`Copied`),
    role: i18n._(msg`Maintainer of Yuva`),
    tryTitle: i18n._(msg`Try Yuva`),
    tryBody: i18n._(msg`Create an account on our service, or run it on your own server.`),
    readTime: (count: number) => i18n._(msg`${count} min read`),
  }
}

export function releasesCopy(i18n: I18n) {
  return {
    title: i18n._(msg`Releases`),
    metaTitle: i18n._(msg`Yuva releases`),
    description: i18n._(msg`Every Yuva release and what changed in it, from the changelog.`),
    lead: i18n._(
      msg`Every version of Yuva and what changed in it. Versions follow Semantic Versioning, and until 1.0 any release may change the API and the database schema.`
    ),
    english: i18n._(msg`Release notes are written in English.`),
    latest: i18n._(msg`Latest`),
    tag: i18n._(msg`Tag on GitHub`),
    compare: i18n._(msg`Compare changes`),
    changelog: i18n._(msg`CHANGELOG.md`),
    all: i18n._(msg`All releases on GitHub`),
    kicker: i18n._(msg`Changelog`),
    track: i18n._(msg`Release history`),
    next: i18n._(msg`Next`),
    nextBody: i18n._(msg`See what is planned`),
    changes: (count: number) => i18n._(msg`${count} changes`),
    kinds: {
      Added: i18n._(msg`Added`),
      Changed: i18n._(msg`Changed`),
      Deprecated: i18n._(msg`Deprecated`),
      Removed: i18n._(msg`Removed`),
      Fixed: i18n._(msg`Fixed`),
      Security: i18n._(msg`Security`),
    } as Record<string, string>,
  }
}
