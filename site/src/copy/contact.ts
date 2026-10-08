import type { I18n } from "@lingui/core"
import { msg } from "@lingui/core/macro"

export function contactCopy(i18n: I18n) {
  return {
    meta: {
      title: i18n._(msg`Contact Yuva`),
      description: i18n._(msg`Write to the Yuva team about investment, partnerships, commercial licenses or press, or find the right place for questions and security reports.`),
    },
    kicker: i18n._(msg`Contact`),
    title: i18n._(msg`Let's talk.`),
    lead: i18n._(
      msg`Whether you want to invest in Yuva, build on it together, need a commercial license or are writing about it, write to us. We read every message.`
    ),
    team: i18n._(msg`Yuva team`),
    about: i18n._(msg`What is it about?`),
    greeting: i18n._(msg`Hi! Tell us a little about yourself and what you have in mind.`),
    topics: [
      { id: "investment", label: i18n._(msg`Investment`), message: i18n._(msg`I'd like to talk about investing in Yuva.`) },
      { id: "partnership", label: i18n._(msg`Partnership`), message: i18n._(msg`I have an idea for working together.`) },
      { id: "license", label: i18n._(msg`Commercial license`), message: i18n._(msg`We would like to use Yuva under a commercial license.`) },
      { id: "press", label: i18n._(msg`Press`), message: i18n._(msg`I'm writing about Yuva and have a few questions.`) },
      { id: "other", label: i18n._(msg`Something else`), message: i18n._(msg`I have a question that doesn't fit anywhere else.`) },
    ],
    write: i18n._(msg`Write an e-mail`),
    copy: i18n._(msg`Copy address`),
    copied: i18n._(msg`Copied`),
    noEmail: i18n._(msg`Open a GitHub issue`),
    investorsTitle: i18n._(msg`For investors`),
    glance: i18n._(msg`At a glance`),
    facts: {
      license: i18n._(msg`License`),
      stack: i18n._(msg`Built with`),
      channels: i18n._(msg`Channels`),
      apps: i18n._(msg`Apps`),
      releases: i18n._(msg`Releases`),
      stage: i18n._(msg`Stage`),
      model: i18n._(msg`Business`),
    },
    channelNames: [i18n._(msg`E-mail`), i18n._(msg`Live chat`), i18n._(msg`In-app`), i18n._(msg`API`)],
    stageValue: i18n._(msg`Pre-alpha`),
    modelValue: i18n._(msg`Hosted service and commercial licenses`),
    releasesValue: (count: number, version: string) => i18n._(msg`${count} releases, latest ${version}`),
    investors: [
      {
        title: i18n._(msg`Open source that stays open`),
        body: i18n._(msg`The server and panel are AGPL-3.0, the SDKs and API contract MIT. Improvements to the core stay public.`),
      },
      {
        title: i18n._(msg`A hosted service and commercial licenses`),
        body: i18n._(msg`Teams can use our hosted service, and because every contribution is made under a CLA, Yuva can also be offered under a commercial license.`),
      },
      {
        title: i18n._(msg`Early and built in the open`),
        body: i18n._(msg`Yuva is pre-alpha. Every release, the roadmap and the code are public, so you can see exactly where it stands.`),
      },
    ],
    elsewhereTitle: i18n._(msg`Other ways to reach us`),
    elsewhereLead: i18n._(msg`Not every message needs an e-mail. Some answers are better in public, and some reports must stay private.`),
    elsewhere: [
      { id: "issues", title: i18n._(msg`Questions and bugs`), body: i18n._(msg`Open an issue on GitHub so others can find the answer too.`), action: i18n._(msg`Open an issue`) },
      { id: "security", title: i18n._(msg`Security`), body: i18n._(msg`Report vulnerabilities privately through GitHub, never in a public issue.`), action: i18n._(msg`Security policy`) },
      { id: "license", title: i18n._(msg`Licensing`), body: i18n._(msg`How the AGPL, the MIT SDKs and the CLA fit together.`), action: i18n._(msg`Read LICENSING.md`) },
      { id: "brand", title: i18n._(msg`Press and brand`), body: i18n._(msg`Logos, colours and the rules for the name.`), action: i18n._(msg`Brand assets`) },
    ],
  }
}

export type ContactCopy = ReturnType<typeof contactCopy>
