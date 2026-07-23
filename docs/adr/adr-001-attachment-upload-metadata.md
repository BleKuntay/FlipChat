# ADR-001: Attachment Upload Metadata Storage

**Status:** Accepted  
**Date:** 2025-07-23  
**Phase:** 2

---

## Decision

Temporary upload records are stored in Redis with the following structure:

```
key:   upload:{attachmentID}
value: {"uploader_id": "...", "mime_type": "...", "size": 1234, "filename": "..."}
TTL:   1 hour
```

---

## Context

`POST /v1/attachments/upload` stores a file in MinIO and returns an `attachment_id` to the client. The client then includes this `attachment_id` when sending a message via `POST /v1/conversations/:id/messages`.

Without a verification record between these two steps, `SendMessage` cannot:

1. Confirm the `attachment_id` actually exists and was uploaded (not guessed or borrowed from another user).
2. Confirm the uploader is the same user sending the message (prevents attachment hijacking).
3. Obtain trusted metadata (`mime_type`, `size`, `filename`) — if read from the client request, the client could send `"mime_type": "text/html"` for a valid JPEG, opening a stored XSS vector at the download endpoint.

Without this record there is also no mechanism to clean up MinIO objects that were uploaded but never attached to a message (orphan objects).

---

## Reasoning

**Why Redis over a PostgreSQL `attachments` table:**

- Redis is already in the stack; no new technology introduced.
- TTL automatically cleans up orphan objects with no sweeper or cron job needed.
- No new DB migration required.
- Ownership verification in `SendMessage` costs a single Redis `GET`.

**Trade-offs consciously accepted:**

- The record is lost if Redis restarts or TTL expires before the user sends the message. In that case the user must re-upload. The error response must be explicit: `"attachment expired, please re-upload"`.
- No standalone upload audit trail — "who uploaded what and when" can only be answered through the `messages` table.
- Redis now holds three roles: presence, refresh tokens, and upload records. A Redis outage affects all three simultaneously.
- TTL is a best guess — 1 hour is chosen as a balance between user convenience and timely orphan cleanup.

**Why not a `attachments` table:**

For Phase 2, the added complexity (migration, JOIN queries, orphan sweeper) is not yet justified. Upload volume is low and an audit trail is not required.

---

## Consequences

- `POST /v1/attachments/upload` writes a record to Redis after successfully storing the file in MinIO.
- `SendMessage` performs a `GET` + `DEL` on the Redis record, verifies `uploader_id == userID`, then copies trusted metadata into the `metadata` JSONB column of the `messages` table. The client no longer sends `mime_type`, `size`, or `filename` — only `attachment_id`.
- The MinIO `object_key` is never returned to the client in any response.
- Orphan objects (uploaded but never attached to a message) are cleaned up automatically when the TTL expires — no sweeper required.

---

## Revisit When (Phase 3)

Migrate to a PostgreSQL `attachments` table if any of the following occur:

- An upload audit trail is needed for content moderation or other purposes.
- Redis needs to shed roles due to memory pressure or reliability concerns.
- There is a need to extend attachment validity without forcing a re-upload (e.g. long-lived message drafts).
- Orphan object volume becomes a real operational problem and a 1-hour TTL is insufficiently flexible.