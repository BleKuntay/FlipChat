# FlipChat WebSocket Protocol

All real-time events are delivered over a single persistent WebSocket connection per user.

## Connection

```
GET /v1/ws?token=<access_token>
```

Pass the access token as a query parameter. Browsers cannot set the
`Authorization` header during the WebSocket handshake.

**One connection per account.** Opening a second connection from the same
account closes the first one silently — the previous tab or device loses
its stream without warning. Design your frontend accordingly.

---

## Message format

All messages (server → client and client → server) share the same envelope:

```json
{
  "type": "<event_type>",
  "payload": { ... }
}
```

---

## Server → Client events

### `message.new`

Delivered to the **recipient** when a new message is sent.

```json
{
  "type": "message.new",
  "payload": {
    "id": "01927f4e-...",
    "conversation_id": "01927f4e-...",
    "sender_id": "01927f4e-...",
    "content": "halo!",
    "reply_to_id": null,
    "metadata": null,
    "is_edited": false,
    "is_deleted": false,
    "created_at": "2025-07-23T10:00:00Z",
    "updated_at": null,
    "read_at": null
  }
}
```

For messages with an attachment, `metadata` is:

```json
{
  "attachment_id": "01927f4e-...",
  "filename": "photo.jpg",
  "mime_type": "image/jpeg",
  "size": 204800
}
```

### `message.edited`

Delivered to the **recipient** when a message is edited.

```json
{
  "type": "message.edited",
  "payload": {
    "id": "01927f4e-...",
    "conversation_id": "01927f4e-...",
    "sender_id": "01927f4e-...",
    "content": "ralat, maksudnya besok ya",
    "reply_to_id": null,
    "metadata": null,
    "is_edited": true,
    "is_deleted": false,
    "created_at": "2025-07-23T10:00:00Z",
    "updated_at": "2025-07-23T10:05:00Z",
    "read_at": null
  }
}
```

### `message.deleted`

Delivered to the **recipient** when a message is deleted. `content` is `null`.

```json
{
  "type": "message.deleted",
  "payload": {
    "id": "01927f4e-...",
    "conversation_id": "01927f4e-...",
    "sender_id": "01927f4e-...",
    "content": null,
    "reply_to_id": null,
    "metadata": null,
    "is_edited": false,
    "is_deleted": true,
    "created_at": "2025-07-23T10:00:00Z",
    "updated_at": null,
    "read_at": null
  }
}
```

### `message.read`

Delivered to the **sender** when the recipient marks a message as read.

```json
{
  "type": "message.read",
  "payload": {
    "id": "01927f4e-...",
    "conversation_id": "01927f4e-...",
    "sender_id": "01927f4e-...",
    "content": "halo!",
    "reply_to_id": null,
    "metadata": null,
    "is_edited": false,
    "is_deleted": false,
    "created_at": "2025-07-23T10:00:00Z",
    "updated_at": null,
    "read_at": "2025-07-23T10:01:00Z"
  }
}
```

### `presence.online`

Delivered to all conversation partners when a user connects.

```json
{
  "type": "presence.online",
  "payload": {
    "user_id": "01927f4e-..."
  }
}
```

### `presence.offline`

Delivered to all conversation partners when a user disconnects.

```json
{
  "type": "presence.offline",
  "payload": {
    "user_id": "01927f4e-..."
  }
}
```

---

## Client → Server events

### `heartbeat`

Send at least once every **30 seconds** to keep the presence record alive.
If the server does not receive a heartbeat within 35 seconds, the user
appears offline to others — even if the WebSocket connection is still open.

```json
{
  "type": "heartbeat"
}
```

The server does not send a response to heartbeat messages.

---

## Connection lifecycle

```
Client                        Server
  |                              |
  |-- GET /v1/ws?token=... ----->|
  |<-- 101 Switching Protocols --|  user marked online, partners notified
  |                              |
  |<-- message.new --------------|  events delivered as they occur
  |-- heartbeat ---------------->|  client sends every ~30s
  |<-- ping (WebSocket level) ---|  server sends every 30s
  |-- pong (WebSocket level) --->|
  |                              |
  |-- close -------------------->|  user marked offline, partners notified
  |<-- close --------------------|
```

**Reconnection:** on disconnect, re-establish the connection and call
`GET /v1/conversations` to catch up on any events missed during the
offline window. There is no event replay mechanism.

---

## Block filtering

Events are silently dropped if either party has blocked the other at the
time of delivery. The connection itself is not terminated on block — only
event delivery is suppressed.

---

## Design decisions

- **One connection per user** (see above). Chosen for simplicity. Phase 3
  may revisit if multi-device support is required.
- **No server-side event replay.** Clients must reconcile via REST on
  reconnect.
- **Heartbeat is client-driven.** The server refreshes presence on each
  heartbeat. A 35-second TTL with a 30-second client interval provides a
  ~5-second grace window for temporary hiccups.