# ADR-003: Read Receipt Granularity

**Status:** Deferred to Phase 3  
**Date:** 2025-07-25  
**Phase:** 2

---

## Decision

Phase 2 implements per-message read receipts via
`POST /conversations/{id}/messages/{msg_id}/read`. The `read_up_to_message_id`
model (mark all messages up to a given ID as read in one request) is deferred
to Phase 3.

---

## Context

The current model requires one HTTP request per unread message. Opening a
conversation with 50 unread messages produces 50 `POST .../read` requests
and 50 `message.read` WebSocket events delivered to the sender.

The `read_up_to_message_id` model works differently:

- One request marks all messages up to (and including) the given message ID
  as read.
- The server infers which messages were unread and updates them in a single
  `UPDATE ... WHERE id <= $1 AND read_at IS NULL`.
- One WebSocket event carries the cursor instead of individual message IDs.

Message IDs are UUIDv7 (time-ordered), making `<=` comparisons on IDs a
correct proxy for time ordering.

---

## Reasoning

**Why per-message was chosen for Phase 2:**

- Simpler to implement and reason about — no cursor semantics.
- Frontend implementation is straightforward: call the endpoint when the
  user scrolls a message into view.
- Acceptable for low-volume conversations typical in a portfolio demo.

**Why `read_up_to_message_id` is better long-term:**

- N requests → 1 request on conversation open.
- N WebSocket events → 1 event carrying a cursor.
- Single DB update instead of N updates — better under load.
- More natural UX model: "I've read everything up to here."

**Why deferred:**

- Changing the model requires a schema change (the per-message `read_at`
  column stays, but the update logic changes) and a frontend contract change.
- Coordinating with the frontend mid-phase adds risk.
- The N+1 problem only becomes painful at scale — not a concern for the
  current portfolio context.

---

## Consequences (current)

Opening a conversation with N unread messages produces N HTTP requests and
N WebSocket events. This is a known inefficiency, not a bug.

---

## Revisit When (Phase 3)

Migrate to `read_up_to_message_id` if any of the following:

- Frontend reports noticeable lag when opening conversations with many
  unread messages.
- Server metrics show `POST .../read` as a significant fraction of request
  volume.
- Group chat is introduced (Phase 3+) — per-message receipts at group
  scale are untenable.

**Migration path:**

1. Add `POST /conversations/{id}/read` with body `{"read_up_to_message_id": "..."}`.
2. Keep per-message endpoint for backward compatibility during transition.
3. Update WebSocket `message.read` event to carry `read_up_to_message_id`
   alongside or instead of individual message IDs.
4. Deprecate and eventually remove per-message endpoint.
