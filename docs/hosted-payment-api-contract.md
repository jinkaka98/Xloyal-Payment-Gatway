# Hosted Payment API Contract

Status: Implemented through Phase 7 payment sessions, events, delivery, hosted checkout, and Custom Web Payment.

Phase 3 runtime routes:

- `POST /v1/payment-sessions` uses `X-API-Key` and derives tenant ownership from the authenticated key.
- `GET /v1/payment-sessions/{token}` is public-token authenticated and returns an explicit public snapshot DTO.
- `POST /v1/payment-sessions/{token}/cancel` is public-token authenticated and idempotent for `CANCELLED` sessions.
- `GET /v1/payment-sessions/{token}/events` is a public-token authenticated SSE stream with sequence cursors.

The service resolves invoice terminal state and request-time session expiry with the Phase 2 atomic transition and event/outbox boundary. SSE and webhook are transports only; payment state is never changed by delivery failures.

This document describes the implemented Phase 4 contract. It intentionally does
not define webhook delivery or a hosted payment UI.

## Authentication Boundaries

Create and cancel requests are server-to-server tenant API calls and use the existing `X-API-Key` convention. Hosted snapshot and SSE endpoints use the opaque public session token and return public-safe data only. The browser must never receive an API key, admin token, provider credential, merchant password, browser credential, or database credential.

## Create Payment Session

```http
POST /v1/payment-sessions
X-API-Key: TENANT_API_KEY
Idempotency-Key: ORDER_RETRY_KEY
Content-Type: application/json
```

Request:

```json
{
  "invoice_id": "inv_opaque_id",
  "theme_id": "theme_opaque_id",
  "success_url": "https://merchant.example/payments/success",
  "cancel_url": "https://merchant.example/payments/cancelled"
}
```

The invoice must belong to the authenticated tenant. The URL fields must match registered tenant redirect destinations. Reusing the same idempotency key with the same request returns the existing session; reusing it with different data returns `409`.

Response `201` or idempotent `200`:

```json
{
  "payment_id": "pay_opaque_id",
  "invoice_id": "inv_opaque_id",
  "session_id": "sess_opaque_id",
  "checkout_url": "https://pay.example/pay/public_random_token",
  "status": "payment_pending",
  "amount": 100037,
  "currency": "IDR",
  "expires_at": "2026-08-18T00:30:00Z"
}
```

The plaintext token is returned only in `checkout_url`; it is never returned as a database identifier or stored plaintext.

**Known limitation:** if the create response is lost, the original random
checkout token cannot be reconstructed safely. Phase 4 does not store plaintext
tokens or add a deterministic-token workaround; callers should retain the
successful response or use their own request correlation until a future
idempotency recovery design is approved.

## Snapshot

```http
GET /v1/payment-sessions/{public_token}
```

Response `200`:

```json
{
  "session_id": "sess_opaque_id",
  "invoice_id": "inv_opaque_id",
  "status": "payment_pending",
  "payment_status": "pending",
  "amount": 100037,
  "currency": "IDR",
  "description": "Order 123",
  "qr_payload": "server_generated_payload",
  "qr_url": "/v1/payment-sessions/token/qr",
  "expires_at": "2026-08-18T00:30:00Z",
  "theme": {
    "template": "modern",
    "version": 3,
    "config": {}
  },
  "redirect": {
    "success_url": "https://merchant.example/payments/success",
    "cancel_url": "https://merchant.example/payments/cancelled"
  }
}
```

The server derives all financial fields. The response omits secrets, internal audit data, provider credentials, and browser session data.

Errors: `404` invalid/unknown token, `410` expired session, `429` rate limit.

## SSE Events

```http
GET /v1/payment-sessions/{public_token}/events?after_sequence=3
Accept: text/event-stream
```

The client first requests the snapshot, then opens SSE. On reconnect it repeats that order.

Event format:

```text
id: 3
event: payment.paid
data: {"event_id":"evt_opaque_id","payment_session_id":"sess_opaque_id","invoice_id":"inv_opaque_id","status":"paid","sequence":3,"occurred_at":"2026-08-18T00:02:00Z"}

```

Event names:

```text
payment.created
payment.pending
payment.verifying
payment.paid
payment.failed
payment.expired
payment.cancelled
payment.redirecting
payment.closed
```

SSE is UX transport only. The snapshot and server-side verification remain authoritative.

The endpoint sends persisted events in increasing `sequence` order, then follows
new events. `after_sequence` resumes with `sequence > cursor`; reconnect should
always perform a fresh snapshot first. Delivery is **at-least-once**, so clients
deduplicate by `event_id` or `sequence`. Heartbeats are SSE comments and are not
payment events. Slow subscribers are disconnected once their bounded transport
buffer is full; the next connection replays persisted events.

## Hosted Payment Page

`/pay/{public_token}` is the customer checkout route. It is public-token based:
the page first resolves the snapshot, renders the server-derived amount/QR/state,
and then connects SSE with the latest persisted sequence. `server_now` in every
snapshot permits an expiry countdown resilient to browser clock skew. The page
uses server-validated success/cancel/failed/expired redirects only; cancellation
waits for the server response so a concurrent paid transition wins correctly.

## Cancel Session

```http
POST /v1/payment-sessions/{public_token}/cancel
```

`PAYMENT_PENDING -> CANCELLED` returns `200` and the latest snapshot. Repeating the request is idempotent. If the session is `PAID`, `EXPIRED`, or `FAILED`, return `409` with the current snapshot. Session cancellation never forces the invoice into `cancelled`.

## Custom Web Payment

`/custom-web-payment` is the existing admin console route for configuring a
tenant payment page. It uses the same `PaymentPageRenderer` as `/pay/{token}`;
the preview changes only its public-safe mock session and viewport. No payment
credential, API key, QR payload generator, or browser session is exposed to
the editor.

The editor stores declarative JSON, up to 64 KB. Accepted keys are
`schema_version`, `template_key`, `branding`, `colors`, `layout`,
`payment_visibility`, `timer`, `success_copy`, and `redirect_delay`. Unknown
fields, HTML, angle brackets, non-HTTPS logo URLs, arbitrary CSS values, and
unsupported template keys are rejected by the API. Color values use `#RRGGBB`.

Theme lifecycle:

```text
DRAFT -> PUBLISHED -> ARCHIVED
ARCHIVED -> DRAFT (by saving a new draft)
```

Publishing validates the draft and atomically creates the next immutable
version. An edit to an already published theme changes its draft only; it
never mutates an older version. Payment sessions retain their original theme
metadata for auditability, but each public checkout snapshot resolves the
tenant's current published default at request time. This means an existing
checkout URL and a newly created URL use the same latest default theme without
issuing a new URL; the payment amount, QR payload, expiry, status, and redirect
metadata remain tied to the original session. An explicit session theme is used
as a fallback only when no published default exists. A tenant has at most one
published default, set atomically. If it has no default, the system default
applies. The Modern, Minimal, Dark, Corporate, and Compact system presets can
be duplicated into a tenant draft but cannot be edited or deleted directly.

Admin routes require the existing bearer token and query/body `tenant_id`:

```text
GET    /admin/payment-themes
POST   /admin/payment-themes
GET    /admin/payment-themes/{id}
PUT    /admin/payment-themes/{id}
DELETE /admin/payment-themes/{id}
POST   /admin/payment-themes/{id}/publish
POST   /admin/payment-themes/{id}/duplicate
POST   /admin/payment-themes/{id}/set-default
POST   /admin/payment-themes/{id}/archive
GET    /admin/payment-themes/{id}/preview
```

`viewer` can list, read, and preview. `operator` can create, edit, duplicate,
publish, archive, and set default. `super_admin` can additionally delete an
unused draft. Theme lookup and mutation are tenant-scoped; a tenant cannot read
or mutate another tenant's theme. Each lifecycle mutation records an audit
event. Theme responses contain only explicit public-safe metadata and config.

## Webhook Contract

When `webhook_url` and an encrypted tenant webhook secret are configured, each
persisted `PaymentEvent` creates one durable delivery identity for that tenant,
event, and endpoint. Delivery is asynchronous and at-least-once. Missing or
invalid webhook configuration never rolls back a payment transition.

Webhook events:

```text
payment.paid
payment.failed
payment.expired
payment.cancelled
```

Payload:

```json
{
  "event_id": "evt_opaque_id",
  "event": "payment.paid",
  "timestamp": 1723939200,
  "payment_id": "pay_opaque_id",
  "invoice_id": "inv_opaque_id",
  "order_id": "merchant-order-123",
  "tenant_id": "tenant_opaque_id",
  "amount": 100037,
  "currency": "IDR",
  "paid_at": "2026-08-18T00:02:00Z"
}
```

Signature:

```text
canonical = <unix_timestamp> + "." + <raw_body>
signature = HMAC-SHA256(WEBHOOK_SECRET, canonical)
```

Headers:

```text
X-Xloyal-Event: payment.paid
X-Xloyal-Timestamp: 1723939200
X-Xloyal-Signature: sha256=<hex_digest>
```

The signature is `HMAC-SHA256(secret, unix_timestamp + "." + raw_body)`.
Receivers should reject timestamps outside the configured five-minute replay
window and deduplicate by `event_id`. Xloyal disables redirect following,
requires HTTPS outside sandbox, rejects loopback/private/link-local/metadata
targets after DNS resolution, and uses a bounded timeout. Responses in the 2xx
range are delivered; 408/429/5xx and network failures retry with bounded
exponential backoff up to 12 attempts. Other 4xx responses are permanent
failures. Delivery state is independent of invoice/payment state.

Delivery uses a configurable timeout, bounded exponential retry, and event-ID idempotency. A delivery failure never changes a persisted payment state.

## HTTP Error Contract

```text
400 invalid request or unregistered redirect
401 missing/invalid tenant API key
403 tenant origin denied
404 resource/token not found
409 invalid lifecycle transition or idempotency conflict
410 session expired/closed and no longer payable
429 rate limit
```

## Acceptance Contract

The implementation is accepted only when it proves:

1. Session creation returns a stable idempotent checkout URL.
2. Hosted page loads amount and status only from the snapshot.
3. Payment engine alone can produce `paid`.
4. `payment.paid` is persisted once and delivered through the SSE transport.
5. Refresh after paid still returns paid without duplicate payment/event.
6. SSE reconnect performs snapshot resynchronization.
7. Cancel before paid changes only the session lifecycle.
8. Expired sessions cannot be paid or reopened.
9. Webhook delivery remains a later phase and is not enabled by this contract.
10. Published theme versions remain immutable for existing sessions.
11. Arbitrary redirects, amount tampering, token guessing, and secret access are rejected.

## Open Decisions Before Phase 2 Coding

1. Confirm whether session creation accepts only `pending` invoices or also a provider-creation `creating` phase.
2. Confirm the public identifier semantics for `payment_id` versus the existing `invoice_id`.
3. Confirm the source and rotation model for tenant webhook secrets; the current repository stores only `webhook_url`.
4. Confirm the webhook replay window, retry schedule, and maximum delivery attempts.
5. Confirm the final theme JSON schema, maximum size, and safe asset URL policy.
6. Confirm whether strict per-payment event ordering requires a persisted sequence number.
