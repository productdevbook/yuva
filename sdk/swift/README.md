# YuvaKit

## Push notifications

Push stays with your app. Yuva sends a `message.created` webhook to your backend; when
`data.contact.online` is false, your backend sends the push itself and puts the conversation's id
(`data.message.conversation_id`) in the payload as `yuva_conversation_id`, a top-level key next to
`aps`:

```json
{
  "aps": {
    "alert": { "title": "Acme Support", "body": "We have shipped a fix." },
    "sound": "default"
  },
  "yuva_conversation_id": "0192f0c4-6f1e-7b2a-9c1d-3e4f5a6b7c8d"
}
```

When the user taps it, pass the notification's `userInfo` to `Yuva.handleNotification` and open
that conversation:

```swift
func userNotificationCenter(
    _ center: UNUserNotificationCenter, didReceive response: UNNotificationResponse
) async {
    if let id = Yuva.handleNotification(response.notification.request.content.userInfo) {
        openConversationId = id
    }
}
```

`YuvaConversationsView(client:openConversationId:)` opens the thread when the binding is set.
