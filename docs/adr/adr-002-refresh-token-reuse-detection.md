# ADR-002: Refresh Token Reuse Detection

**Status:** Deferred to Phase 3  
**Date:** 2025-07-25  
**Phase:** 2

---

## Decision

Refresh token reuse detection is not implemented in Phase 2. A reused
token is rejected (the old token no longer exists after rotation), but
the system does not treat reuse as a signal of token theft and does not
revoke all sessions for the affected user.

---

## Context

The current token rotation flow in `auth.Repository.RotateRefreshToken`:

1. Client sends old refresh token.
2. Server validates and deletes the old token key.
3. Server issues a new token.

If an attacker steals a refresh token and uses it before the legitimate
user does, one of two things happens:

- **Attacker uses it first** — attacker gets a new token, legitimate
  user's next refresh fails with `ErrNotFound`. The legitimate user is
  effectively logged out but the attacker retains access.
- **Legitimate user uses it first** — attacker's copy is now stale and
  will be rejected. No harm done.

The first scenario is the dangerous one. The correct response is to treat
any reuse of an already-rotated token as evidence of compromise and revoke
all sessions for that user — forcing the attacker out along with the
legitimate user, who then re-authenticates.

---

## Reasoning

**Why deferred:**

- Requires storing the previous token (or a hash of it) after rotation so
  the server can distinguish "token not found" (expired/never existed) from
  "token already rotated" (possible reuse). This adds complexity to the
  rotation flow.
- Alternatively, a separate Redis key (e.g. `rotated:{oldToken}` with short
  TTL) can track recently rotated tokens cheaply — but this is new
  infrastructure logic.
- Phase 2 already covers the primary security concern: password change and
  account deletion revoke all sessions. Reuse detection is defense in depth,
  not the first line.
- `DeleteTokenByUserID` is already available and would be the revocation
  mechanism — the detection logic is what is missing, not the revocation.

---

## Consequences (current)

A stolen refresh token can be used by an attacker until it expires
naturally, provided the legitimate user does not trigger a refresh in the
meantime. Access token lifetime (15 minutes default) limits the blast
radius between token theft and detection.

---

## Revisit When (Phase 3)

Implement reuse detection if any of the following:

- A security review flags token theft as a realistic threat for the
  deployment environment.
- The access token lifetime is increased beyond 15 minutes, widening the
  window between theft and detection.
- User-facing "active sessions" or "sign out everywhere" features are
  added, making the session model more visible and therefore more important
  to get right.

**Implementation sketch:**
```
On RotateRefreshToken:
  1. Check if tokenKey(oldToken) exists.
  2. If not found, check rotatedKey(oldToken) — if present, this is reuse.
     → DeleteTokenByUserID (revoke all) + return ErrUnauthorized.
  3. If not found and no rotated record — token expired normally.
     → return ErrNotFound.
  4. If found — proceed with rotation, write rotatedKey(oldToken) with
     short TTL (e.g. 5 minutes) as a breadcrumb.
```
