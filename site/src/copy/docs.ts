import type { I18n } from "@lingui/core"
import { msg } from "@lingui/core/macro"

export function docsCopy(i18n: I18n) {
  return {
    docs: i18n._(msg`Docs`),
    guides: i18n._(msg`Guides`),
    copy: i18n._(msg`Copy`),
    copied: i18n._(msg`Copied`),
    backToSite: i18n._(msg`Back to the website`),
    documentation: i18n._(msg`Documentation`),
    english: i18n._(msg`This documentation is written in English.`),
    searchDocs: i18n._(msg`Search docs`),
    searchTheDocs: i18n._(msg`Search the docs`),
    noResults: i18n._(msg`No results`),
    searchHint: i18n._(msg`Type to search the guides and the API reference`),
    onThisPage: i18n._(msg`On this page`),
    lastUpdated: (date: string) => i18n._(msg`Last updated ${date}`),
    readTime: (count: number) => i18n._(msg`${count} min read`),
    sectionCount: (count: number) => i18n._(msg`${count} sections`),
    edit: i18n._(msg`Edit this page on GitHub`),
    previous: i18n._(msg`Previous`),
    next: i18n._(msg`Next`),
    groups: {
      start: i18n._(msg`Get started`),
      channels: i18n._(msg`Channels`),
      integrate: i18n._(msg`Integrate`),
      project: i18n._(msg`Project`),
      more: i18n._(msg`More`),
    },
    apiReference: i18n._(msg`API reference`),
    webhookEvents: i18n._(msg`Webhook events`),
    home: {
      title: i18n._(msg`Yuva documentation`),
      description: i18n._(
        msg`Install, configure and integrate Yuva: e-mail, the web widget, the mobile SDKs, identity tokens, webhooks and the API reference.`
      ),
      heading: i18n._(msg`Everything to run and build on Yuva.`),
      lead: i18n._(
        msg`Install the server, connect your channels and integrate the widget, the mobile SDKs and the API. Written for the version on the main branch.`
      ),
      popular: i18n._(msg`Popular`),
      quickStart: i18n._(msg`Quick start`),
      steps: {
        start: { title: i18n._(msg`Run Yuva`), body: i18n._(msg`One Docker image and Postgres, from compose file to the first owner.`) },
        channels: {
          title: i18n._(msg`Connect your channels`),
          body: i18n._(msg`Your support e-mail, live chat on your sites and messages inside your apps, in one inbox.`),
        },
        integrate: {
          title: i18n._(msg`Integrate with your backend`),
          body: i18n._(msg`Let your users write as themselves, react to events and automate the rest with the API.`),
        },
      } as Record<string, { title: string; body: string }>,
    },
    api: {
      description: i18n._(msg`Every Yuva endpoint and webhook event, generated from the OpenAPI 3.1 contract.`),
      lead: (version: string) => i18n._(msg`Version ${version} of the API, generated from the OpenAPI contract. Use the contract to generate a client in any language.`),
      contract: i18n._(msg`OpenAPI contract`),
      overview: i18n._(msg`Overview`),
      authentication: i18n._(msg`Authentication`),
      resources: i18n._(msg`Resources`),
      eventsCard: i18n._(msg`The payloads Yuva sends to your webhook endpoints.`),
      tagFallback: (name: string) => i18n._(msg`${name} endpoints of the Yuva API.`),
      parameters: i18n._(msg`Parameters`),
      requestBody: i18n._(msg`Request body`),
      responses: i18n._(msg`Responses`),
      request: i18n._(msg`Request`),
      response: i18n._(msg`Response`),
      noContent: i18n._(msg`No content`),
      payload: i18n._(msg`Payload`),
      columns: { name: i18n._(msg`Name`), in: i18n._(msg`In`), type: i18n._(msg`Type`), description: i18n._(msg`Description`), required: i18n._(msg`required`) },
      auth: {
        session: i18n._(msg`Member session`),
        apiKey: i18n._(msg`API key`),
        contactSession: i18n._(msg`Contact session`),
        none: i18n._(msg`No authentication`),
      } as Record<string, string>,
      schemas: i18n._(msg`Schemas and every field are in the OpenAPI contract.`),
      eventsDescription: i18n._(msg`The events Yuva sends to your webhook endpoints and the payload of each.`),
      eventsLead: i18n._(msg`Yuva signs every request with Standard Webhooks and retries failed deliveries. Registering an endpoint and verifying a signature are in the webhooks guide.`),
      webhooksGuide: i18n._(msg`Webhooks guide`),
    },
  }
}

export type DocsCopy = ReturnType<typeof docsCopy>
