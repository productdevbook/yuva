import { useLingui } from "@lingui/react/macro"

import type {
  MigrationFileFormat,
  MigrationFolderKind,
  MigrationPresetId,
  MigrationSignInResult,
  MigrationSource,
  MigrationState,
  MigrationTestCode,
  MigrationTestStep,
} from "@/lib/api"

export const DOCS = "https://github.com/productdevbook/yuva/blob/main/docs"

export const TEST_STEPS: MigrationTestStep["step"][] = ["resolve", "connect", "tls", "greeting", "sign_in", "folders"]

export const NEVER_READ: MigrationFolderKind[] = ["spam", "trash", "drafts", "other"]

export type Tone = "muted" | "success" | "warning" | "danger"

export const stateTones: Record<MigrationState, Tone> = {
  draft: "muted",
  connected: "muted",
  scanning: "warning",
  ready: "warning",
  copying: "warning",
  paused: "muted",
  needs_attention: "danger",
  syncing: "warning",
  completed: "success",
  cancelled: "muted",
  removing: "warning",
  removed: "muted",
}

export function isHelpdesk(source: MigrationSource | undefined) {
  return source !== undefined && source !== "imap" && source !== "file"
}

const TLDISH = /^[a-z]{2,3}(\.[a-z]{2,3})?$/

export function isPersonalMicrosoft(address: string) {
  const domain = address.split("@")[1]?.toLowerCase() ?? ""
  const [first = "", ...rest] = domain.split(".")
  return domain === "msn.com" || (["outlook", "hotmail", "live"].includes(first) && TLDISH.test(rest.join(".")))
}

export function guideUrl(slug: string) {
  return `${DOCS}/${slug}.md`
}

export function usePresetName() {
  const { t } = useLingui()
  const names: Partial<Record<MigrationPresetId, string>> = {
    microsoft: t`Outlook.com and Microsoft 365`,
    imap: t`Other IMAP server`,
    file: t`Export file`,
  }
  return (id: MigrationPresetId, name: string) => names[id] ?? name
}

export function useMigrationText() {
  const { t } = useLingui()
  return {
    state: {
      draft: t({ message: "Draft", context: "migration state" }),
      connected: t`Connected`,
      scanning: t`Scanning`,
      ready: t`Ready`,
      copying: t`Copying`,
      paused: t`Paused`,
      needs_attention: t`Needs attention`,
      syncing: t`Syncing`,
      completed: t`Completed`,
      cancelled: t`Cancelled`,
      removing: t`Removing history`,
      removed: t`History removed`,
    } satisfies Record<MigrationState, string>,
    step: {
      resolve: t`Find the server`,
      connect: t`Connect`,
      tls: t`Encryption`,
      greeting: t`Server answers`,
      sign_in: t`Sign in`,
      folders: t`Read the folders`,
    } satisfies Record<MigrationTestStep["step"], string>,
    folderKind: {
      inbox: t`Inbox`,
      sent: t`Sent mail`,
      archive: t({ message: "Archive", context: "mail folder kind" }),
      all: t`All mail`,
      user: t`Your folder`,
      label: t`Label`,
      spam: t`Spam, never imported`,
      trash: t`Trash, never imported`,
      drafts: t`Drafts, never imported`,
      other: t`Never imported`,
    } satisfies Record<MigrationFolderKind, string>,
  }
}

export function useTestCodeText() {
  const { t } = useLingui()
  return (code: MigrationTestCode, provider: string, host: string) =>
    ({
      dns_failed: t`${host} does not resolve. Check the server's name.`,
      address_not_allowed: t`${host} is on a private or local network, which this server does not connect to.`,
      connect_failed: t`Nothing answers on that port of ${host}. Check the port and that IMAP is on.`,
      tls_failed: t`The server's certificate is not valid for ${host}. If it is your own server with its own certificate, you can trust it below.`,
      not_imap: t`The server answered, but not as an IMAP server. Check the host and port.`,
      starttls_unavailable: t`The server offers no STARTTLS. Use TLS on port 993 instead.`,
      auth_failed: t`${provider} did not accept the user name or password. Check both, or create a new app password.`,
      app_password_required: t`${provider} wants an app password, not your account's password. Create one with the steps above.`,
      imap_disabled: t`IMAP is off for this mailbox. Turn it on in ${provider}'s settings, then test again.`,
      basic_auth_disabled: t`${provider} accepts no passwords over IMAP. This mailbox connects only with “Sign in with Microsoft”: go back and pick Outlook.com and Microsoft 365.`,
      web_login_required: t`${provider} wants you to sign in once in the browser. Sign in to your mail on the web, then test again.`,
      too_many_connections: t`The mailbox has too many connections open. Close a mail app or wait a few minutes, then test again.`,
      throttled: t`${provider} limits this account for now. Wait a while, then test again.`,
      no_folders: t`The mailbox lists no folders.`,
      examine_failed: t`The inbox could not be opened read-only.`,
      timeout: t`The test ran out of its 20 seconds. The server may be slow; try again.`,
      unexpected_answer: t`The server gave an answer Yuva did not expect. The details below show it.`,
      not_admin: t`The ${provider} user you signed in with is not an admin, and only admins can read every ticket. Sign in again as an admin.`,
      plan_has_no_api: t`This ${provider} account's plan has no API access, which Yuva reads through.`,
      subdomain_not_found: t`No ${provider} account has that subdomain. Check it in your address: acme in acme.zendesk.com.`,
      xoauth2_failed: t`The mailbox refused Microsoft's sign-in over IMAP. Usually IMAP is turned off for it: an admin turns it on in the Microsoft 365 admin center › Users › Active users › the mailbox › Mail › Manage email apps. Then sign in again.`,
      token_refresh_failed: t`Microsoft no longer accepts Yuva's sign-in: access was removed, the password changed, or it went unused too long. Sign in with Microsoft again.`,
      scope_too_broad: t`Microsoft's token grants more than reading mail over IMAP, so Yuva does not use it. Check the app's API permissions, then sign in again.`,
    })[code]
}

export type Guide = { steps: string[]; link: string; linkLabel: string }

export function useGuides() {
  const { t } = useLingui()
  const google = [
    t`Turn on 2-Step Verification for the Google account, if it is not on yet.`,
    t`Open App passwords in the Google account, name the password “Yuva” and create it.`,
    t`Copy the 16-character password and paste it below. Gmail's IMAP is always on.`,
  ]
  return {
    gmail: {
      steps: google,
      link: "https://myaccount.google.com/apppasswords",
      linkLabel: t`Open App passwords`,
    },
    google_workspace: {
      steps: [
        ...google,
        t`No App passwords page? Your admin must allow 2-Step Verification and IMAP in the Google Admin console.`,
      ],
      link: "https://myaccount.google.com/apppasswords",
      linkLabel: t`Open App passwords`,
    },
    microsoft: {
      steps: [t`Outlook.com and Microsoft 365 accept no passwords. Yuva connects with “Sign in with Microsoft”.`],
      link: guideUrl("migrate-microsoft"),
      linkLabel: t`Read the guide`,
    },
    yandex: {
      steps: [
        t`In Yandex Mail settings, under Mail clients, allow access over IMAP with app passwords.`,
        t`In Yandex ID, open Security › App passwords and create a password for Mail, named “Yuva”.`,
        t`Copy it and paste it below.`,
      ],
      link: "https://id.yandex.com/security/app-passwords",
      linkLabel: t`Open App passwords`,
    },
    icloud: {
      steps: [
        t`Sign in to your Apple Account and open Sign-In and Security › App-Specific Passwords.`,
        t`Generate a password named “Yuva” and paste it below.`,
        t`The user name is your iCloud address without @icloud.com.`,
      ],
      link: "https://account.apple.com",
      linkLabel: t`Open your Apple Account`,
    },
    zoho: {
      steps: [
        t`In Zoho Mail, open Settings › Mail accounts › IMAP access and turn IMAP on.`,
        t`With two-factor sign-in, create an app password in Zoho Accounts › Security › App passwords; otherwise your password works.`,
        t`Pick the server of your data centre under Server settings.`,
      ],
      link: "https://accounts.zoho.com",
      linkLabel: t`Open Zoho Accounts`,
    },
    yahoo: {
      steps: [
        t`Open Account security in your Yahoo account.`,
        t`Choose “Generate app password”, name it “Yuva” and create it.`,
        t`Copy it and paste it below.`,
      ],
      link: "https://login.yahoo.com/account/security",
      linkLabel: t`Open Account security`,
    },
    fastmail: {
      steps: [
        t`In Fastmail, open Settings › Privacy & Security › Manage app passwords.`,
        t`Create a new app password with access to mail over IMAP, named “Yuva”.`,
        t`Copy it and paste it below.`,
      ],
      link: "https://app.fastmail.com/settings/security",
      linkLabel: t`Open Privacy & Security`,
    },
    gmx: {
      steps: [
        t`In GMX, open E-mail settings › POP3 & IMAP and allow access over IMAP.`,
        t`Use your GMX password, or an app password if two-factor sign-in is on.`,
        t`Pick the server for your address under Server settings: gmx.net, gmx.com or mail.com.`,
      ],
      link: guideUrl("migrate-gmx"),
      linkLabel: t`Read the guide`,
    },
    imap: {
      steps: [
        t`Find the IMAP server of your mail in your provider's help or your mail app's account settings: a host, port 993 with TLS or 143 with STARTTLS.`,
        t`Use the mailbox's user name and password, or an app password if your provider offers them.`,
      ],
      link: guideUrl("migrate-imap"),
      linkLabel: t`Read the guide`,
    },
    takeout: {
      steps: [
        t`Open Google Takeout, choose “Deselect all”, then select only Mail.`,
        t`Export once as a .zip file. Google mails you a link when it is ready; for a large mailbox that can take a day or more.`,
        t`Download and unzip it, then upload the .mbox file from the Takeout › Mail folder. The .mbox file inside the .zip is compressed and cannot be read from there.`,
      ],
      link: "https://takeout.google.com/settings/takeout/custom/gmail",
      linkLabel: t`Open Google Takeout`,
    },
    zendesk: {
      steps: [
        t`Sign in to Zendesk as an admin and open Admin Center › Apps and integrations › APIs › OAuth clients.`,
        t`Add an OAuth client named “Yuva”, as a confidential client, and enter the redirect URL below as its redirect URL.`,
        t`Save it and copy its identifier and the secret, which Zendesk shows only once.`,
        t`Paste them below with your subdomain. Yuva asks only for the scope read and accepts nothing more.`,
      ],
      link: guideUrl("migrate-zendesk"),
      linkLabel: t`Read the guide`,
    },
    helpscout: {
      steps: [
        t`In Help Scout, open Your Profile › My Apps and choose Create App.`,
        t`Name it “Yuva” and enter the redirect URL below as its redirection URL.`,
        t`Create it, then copy its App ID and App Secret and paste them below.`,
        t`Sign in as a user who sees every mailbox you want to move.`,
      ],
      link: guideUrl("migrate-helpscout"),
      linkLabel: t`Read the guide`,
    },
    chatwoot: {
      steps: [
        t`In Chatwoot, open Profile Settings (your avatar, bottom left), scroll to Access Token and copy it. Use an administrator: an agent's token is refused.`,
        t`Find the account number in Chatwoot's address: the number after /app/accounts/.`,
        t`Enter the address of your Chatwoot, the account number and the token below. Reset the token in Chatwoot when the migration is done.`,
      ],
      link: guideUrl("migrate-chatwoot"),
      linkLabel: t`Read the guide`,
    },
    front: {
      steps: [
        t`In Front, open Settings › Developers › API tokens and choose Create API token. Name it “Yuva”.`,
        t`Choose the feature Access resources, the namespaces Global resources and Shared resources (add Private resources only if personal inboxes should move), and the permission Read only.`,
        t`Create it, then copy the token, which Front shows only once, and paste it below.`,
      ],
      link: guideUrl("migrate-front"),
      linkLabel: t`Read the guide`,
    },
    freshdesk: {
      steps: [
        t`Sign in to Freshdesk as an admin, open your profile picture at the top right and choose Profile settings.`,
        t`Copy the key under “Your API Key”. A Freshdesk key cannot be limited to reading; Yuva's connection only sends read requests.`,
        t`Paste it below with your subdomain, the acme of acme.freshdesk.com.`,
      ],
      link: guideUrl("migrate-freshdesk"),
      linkLabel: t`Read the guide`,
    },
    intercom: {
      steps: [
        t`In Intercom, open Settings › Integrations › Developer Hub, choose New app and name it “Yuva”. Create it in your own workspace.`,
        t`Under Configure › Authentication, turn on only the read permissions: read conversations, read users and companies, read admins and read tags.`,
        t`Copy the access token from the same page and paste it below with your workspace's region. Delete the app when the migration is done.`,
      ],
      link: guideUrl("migrate-intercom"),
      linkLabel: t`Read the guide`,
    },
    crisp: {
      steps: [
        t`In Crisp, open Settings › Workspace Settings › Advanced configuration › API Token and generate a token. Paste its identifier and key below, with the website ID from the address of your Crisp dashboard, after /website/.`,
      ],
      link: guideUrl("migrate-crisp"),
      linkLabel: t`Read the guide`,
    },
    file: {
      steps: [
        t`Thunderbird, Apple Mail and most mail apps export a folder as an .mbox file: upload that file.`,
        t`Folders of .eml files, a Maildir or Proton Mail's export: put the folder in a .zip and upload it. Its folders become labels.`,
        t`Outlook's .pst files are not read; convert them to .mbox first.`,
      ],
      link: guideUrl("migrate-files"),
      linkLabel: t`Read the guide`,
    },
  } satisfies Record<MigrationPresetId, Guide>
}

export function useMicrosoftAppSteps() {
  const { t } = useLingui()
  return (personal: boolean) => [
    t`Open the Microsoft Entra admin center or the Azure portal, go to App registrations and choose New registration. Name it “Yuva”.`,
    personal
      ? t`Under Supported account types, choose personal Microsoft accounts only, for Outlook.com, Hotmail and Live.`
      : t`Under Supported account types, choose accounts in this organizational directory only for your organization's mailboxes, or any organizational directory and personal accounts if you are not sure.`,
    t`Under Redirect URI, choose Web and enter the redirect URL below.`,
    t`In Certificates & secrets, add a new client secret and copy its Value, which is shown only once.`,
    t`In API permissions, add a permission: APIs my organization uses › Office 365 Exchange Online › Delegated permissions › IMAP.AccessAsUser.All. Add offline_access from Microsoft Graph too.`,
    t`Copy the Application (client) ID from Overview and paste it below with the secret. For an app limited to your organization, also enter its Directory (tenant) ID.`,
  ]
}

export type SignInApp = "server" | "team"

export function useSignInResultText() {
  const { t } = useLingui()
  return (result: MigrationSignInResult, provider: string, microsoft?: SignInApp) => {
    if (microsoft) {
      const own = microsoft === "team"
      const texts: Partial<Record<MigrationSignInResult, { title: string; text: string }>> = {
        scope_too_broad: {
          title: t`Microsoft granted more than IMAP access`,
          text: own
            ? t`Yuva stored nothing. Give the app registration only the permissions IMAP.AccessAsUser.All and offline_access, then sign in again.`
            : t`Yuva stored nothing. Sign in again; if this keeps happening, tell the operator of this Yuva server.`,
        },
        account_mismatch: {
          title: t`Another account`,
          text: t`The Microsoft account you signed in with cannot open this mailbox, and Yuva stored nothing. Sign in with the mailbox's own account; use a private window if the browser keeps choosing the other one.`,
        },
        token_failed: {
          title: t`Microsoft refused the sign-in`,
          text: own
            ? t`Usually the client secret is wrong or expired, or the redirect URL is not a Web redirect URI of the app registration. Check both, then sign in again.`
            : t`Try again; if this keeps happening, tell the operator of this Yuva server.`,
        },
      }
      const text = texts[result]
      if (text) return text
    }
    return (
      {
        connected: {
          title: t`Signed in to ${provider}`,
          text: t`Yuva can read the account now. Next, choose what to bring.`,
        },
        access_denied: {
          title: t`The sign-in was declined`,
          text: t`${provider} did not give Yuva access. Sign in again and allow access on ${provider}'s page.`,
        },
        expired: {
          title: t`The sign-in expired`,
          text: t`A sign-in must come back within 10 minutes and works once. Sign in again.`,
        },
        session_mismatch: {
          title: t`The sign-in came back in another browser`,
          text: t`It must end in the browser of the member who started it. Start it again here and finish it in this browser.`,
        },
        scope_too_broad: {
          title: t`${provider} granted more than read access`,
          text: t`Yuva stored nothing. Make sure the OAuth client asks for nothing more than read, then sign in again.`,
        },
        not_admin: {
          title: t`Not an admin`,
          text: t`The ${provider} user you signed in with is not an admin, and only admins can read every ticket. Sign in again as an admin.`,
        },
        plan_has_no_api: {
          title: t`No API on this plan`,
          text: t`This ${provider} account's plan has no API access, which Yuva reads through. A plan with API access is needed to move.`,
        },
        subdomain_not_found: {
          title: t`Subdomain not found`,
          text: t`No ${provider} account has that subdomain. Check it in your address: acme in acme.zendesk.com.`,
        },
        account_mismatch: {
          title: t`Another account`,
          text: t`You signed in to another ${provider} account than the one this migration reads. Sign in to the same account to continue.`,
        },
        token_failed: {
          title: t`${provider} refused the sign-in`,
          text: t`Usually the client secret, or the redirect URL in the OAuth client, is wrong. Check both, then sign in again.`,
        },
        source_unreachable: {
          title: t`${provider} could not be reached`,
          text: t`Try again in a few minutes.`,
        },
        consent_required: {
          title: t`Microsoft wants your consent again`,
          text: t`Sign in again and accept on Microsoft's page.`,
        },
        admin_consent_required: {
          title: t`Your organization's admin must allow Yuva`,
          text: t`Your Microsoft 365 organization lets only an admin consent to apps. Send your admin the link below; once they accept, sign in again.`,
        },
        conditional_access: {
          title: t`Your organization blocked the sign-in`,
          text: t`A Conditional Access policy of your Microsoft 365 organization does not allow this sign-in. Ask your admin to allow it, or upload an export file of the mailbox instead.`,
        },
        xoauth2_failed: {
          title: t`The mailbox refused the sign-in over IMAP`,
          text: t`Microsoft signed you in, but the mailbox refused the token: usually IMAP is turned off for it. Ask your admin to turn it on in the Microsoft 365 admin center › Users › Active users › the mailbox › Mail › Manage email apps, then sign in again.`,
        },
        admin_consented: {
          title: t`Your admin allowed Yuva`,
          text: t`Sign in with Microsoft again to connect the mailbox.`,
        },
      } satisfies Record<MigrationSignInResult, { title: string; text: string }>
    )[result]
  }
}

export function useFormatText() {
  const { t } = useLingui()
  return {
    takeout: t`Read as a Google Takeout file: Gmail's labels and threads come with it.`,
    mbox: t`Read as an mbox file.`,
    mbox_zip: t`Read as a zip of mbox files: each file becomes a label.`,
    eml_zip: t`Read as a zip of .eml files: its folders become labels.`,
    maildir_zip: t`Read as a zipped Maildir: its folders become labels.`,
  } satisfies Record<MigrationFileFormat, string>
}

export function useFormatName() {
  const { t } = useLingui()
  return {
    takeout: t`Google Takeout`,
    mbox: "mbox",
    mbox_zip: t`mbox files in a zip`,
    eml_zip: t`.eml files in a zip`,
    maildir_zip: t`Maildir in a zip`,
  } satisfies Record<MigrationFileFormat, string>
}

export function useItemErrorText() {
  const { t } = useLingui()
  const texts: Record<string, string> = {
    too_large: t`Larger than this server's limit for one message`,
    unparseable: t`The message could not be read`,
    gone: t`No longer in the mailbox`,
    failed: t`Failed five times`,
    blocked: t`From a blocked contact`,
    auth_failed: t`The mailbox refused the sign-in`,
    source_unreachable: t`The mail server could not be reached`,
    throttled: t`The provider asked to slow down`,
    folder_changed: t`The folder was recreated and read again`,
    hosted_limit: t`This server's limit was reached`,
    source_write_refused: t`Yuva held back a command that would have changed the mailbox`,
    token_refresh_failed: t`Microsoft no longer accepts the sign-in`,
    xoauth2_failed: t`The mailbox refused the sign-in over IMAP`,
  }
  return (code: string) => texts[code]
}
