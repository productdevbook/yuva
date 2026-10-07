# Yuva for Android

## Push notifications

Push stays with your app. Yuva sends a `message.created` webhook to your backend; when
`data.contact.online` is false, your backend sends the push itself and puts the conversation's id
(`data.message.conversation_id`) in the FCM message as the `data` entry `yuva_conversation_id`:

```json
{
  "message": {
    "token": "<device registration token>",
    "notification": { "title": "Acme Support", "body": "We have shipped a fix." },
    "data": { "yuva_conversation_id": "0192f0c4-6f1e-7b2a-9c1d-3e4f5a6b7c8d" }
  }
}
```

When the user taps a notification, Android starts your launcher activity with the `data` entries
as intent extras; pass them to `Yuva.handleNotification` and open that conversation:

```kotlin
val extras = intent.extras?.keySet()?.associateWith { intent.extras?.getString(it).orEmpty() }.orEmpty()
val conversationId = Yuva.handleNotification(extras)
```

In `FirebaseMessagingService.onMessageReceived`, `Yuva.handleNotification(message.data)` works the
same way. `YuvaConversationsView(client, openConversationId = conversationId)` opens the thread.
