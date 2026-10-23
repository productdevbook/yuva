# Slack

Yuva can post an inbox's new conversations and the notes that mention members into Slack channels.
Members answer in the post's thread: a reply becomes a note in the conversation, and a reply that
starts with `!reply` goes to the customer.

## Create the Slack app

The server's operator does this once; every workspace on the server connects through the same app.

1. At [api.slack.com/apps](https://api.slack.com/apps), choose **Create New App** › **From a
   manifest**, pick a Slack workspace to develop it in, and paste this manifest with your server's
   address in place of `support.example.com`:

   ```yaml
   display_information:
     name: Yuva
   features:
     bot_user:
       display_name: Yuva
       always_online: true
   oauth_config:
     redirect_urls:
       - https://support.example.com/slack/oauth/callback
     scopes:
       bot:
         - chat:write
         - channels:read
         - groups:read
         - channels:history
         - groups:history
         - im:write
         - users:read
         - users:read.email
   settings:
     event_subscriptions:
       request_url: https://support.example.com/ingress/slack
       bot_events:
         - message.channels
         - message.groups
         - app_uninstalled
         - tokens_revoked
     org_deploy_enabled: false
     socket_mode_enabled: false
     token_rotation_enabled: false
   ```

2. Under **Basic Information**, copy the Client ID, the Client Secret and the Signing Secret into
   the server's configuration and restart it:

   ```sh
   YUVA_SLACK_CLIENT_ID=1234567890.1234567890
   YUVA_SLACK_CLIENT_SECRET=...
   YUVA_SLACK_SIGNING_SECRET=...
   ```

3. Slack checks the request URL once the server answers there; on the app's **Event
   Subscriptions** page, retry the check if it was saved before the server had the signing secret.
4. To let workspaces outside the one you developed the app in connect, turn on **Manage
   Distribution** › **Public Distribution**.

What the scopes are for: `chat:write` posts and answers, `channels:read` and `groups:read` list
the channels the bot is in, `channels:history` and `groups:history` deliver the replies in
threads, `im:write` sends the link code, and `users:read` and `users:read.email` find a member's
Slack user by e-mail address. Yuva reads no channel history; it only receives the messages Slack
sends it as events, and acts only on replies in threads it posted.

Without the three variables Slack is off: the panel does not offer it and the API answers
`409 slack_not_configured`.

## Connect a workspace

An owner or admin opens Settings › Slack and chooses **Connect Slack**, then allows the app in
Slack. One Slack workspace is connected to one Yuva workspace at a time. The bot token is stored
encrypted under the server's master key.

Invite the bot into each channel it should post in: `/invite @Yuva` in that channel. Private
channels work the same way.

## Choose channels per inbox

In an inbox's settings, pick:

- a channel for **new conversations** (a contact's first message), and
- a channel for **mentions** (a note that mentions members; a linked member is mentioned in Slack
  too), and
- a channel for **SLA** (a conversation breaches an SLA target; switch on **SLA warnings** to also
  post when a target reaches its warning). The post names the target (first reply, next reply or
  resolution), how late it is (or how long is left), the assignee or team, the subject, the contact
  and a link.

A post carries the subject, the contact's name, the inbox and a link to the conversation. The
message itself stays in Yuva unless the inbox turns on **Include message text**; then the post
also quotes its first 300 characters. A mention or SLA post goes into the conversation's thread when the
channel already has one, and a reply there works like in any other thread.

With the API: `PUT /v1/inboxes/{id}/slack` with `new_conversations_channel_id`,
`mentions_channel_id`, `sla_channel_id`, `sla_warnings` and `include_content`; `GET /v1/slack/channels` lists the channels the bot
is in.

## Link your Slack user

Each member links their own Slack user under their profile: Yuva finds the Slack user with the
member's e-mail address (or another address you enter) and the Yuva bot sends them a 6-digit code
in Slack, valid for 10 minutes. Entering the code links the two. A Slack user belongs to one
member at a time.

## Reply from Slack

In the thread of a post:

- any reply becomes a **note** in the conversation, written by the linked member;
- a reply that starts with `!reply` goes to the **customer** as that member's reply, sent the
  usual way (e-mail, chat or app). An owner or admin can choose another command, such as `!send`;
  it cannot start with `/`, which Slack keeps for its own commands.

Both show "via Slack" in Yuva. Links keep their address; mentions of linked Slack users become the
members' names.

Nothing happens for a Slack user who has not linked a member, or whose member cannot see the
conversation's inbox: they get a message only they can see that says why. Edits and deletions in
Slack are not carried over.

## Disconnect

Settings › Slack › **Disconnect** revokes the bot token at Slack and removes the inboxes' channels,
the threads and the members' links. Removing the app in Slack does the same. Connecting and
disconnecting, and members linking and unlinking, are in the workspace's audit log.
