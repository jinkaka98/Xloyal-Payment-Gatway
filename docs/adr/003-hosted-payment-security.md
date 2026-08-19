# ADR 003: Hosted Payment Security

Status: Accepted for Phase 2 design

## Public Token

The browser receives a cryptographically random opaque token. PostgreSQL stores only a SHA-256 hash of the token (or an equivalent keyed lookup digest if the implementation requires it). Token lookup is tenant-independent and returns only public-safe session data.

Tokens must have sufficient entropy, must not be derived from database IDs or payment data, and must be rate limited. Logs must never contain the plaintext token when it can identify an active payment.

## Amount and Status Integrity

Amount, currency, invoice status, expiry, and merchant display data come from the server snapshot. Query parameters are never accepted as amount or payment status input. The browser cannot mark an invoice or session paid.

## Redirect Policy

`success_url`, `cancel_url`, `failed_url`, and `expired_url` must resolve against a tenant allow-list. Phase 2 uses exact URL matching against `tenant_allowed_redirect_urls`; origin-only matching is not sufficient for a destination that contains a path.

An unregistered URL rejects session creation with `400`. Existing sessions retain their validated destinations. Fragments and query strings are part of the exact comparison policy and must be normalized consistently.

## SSE

SSE is public only through a valid session token. It exposes payment/session events and public-safe display fields. It does not expose tenant secrets, audit events, provider credentials, browser credentials, or internal errors. Connections are rate limited and bounded per token/IP.

The snapshot endpoint is authoritative. On connect and reconnect, the client fetches the snapshot before opening SSE.

## Webhook

Webhook payloads are signed with HMAC using the tenant webhook secret. The canonical string is:

```text
<unix_timestamp>.<raw_request_body>
```

Headers:

```text
X-Xloyal-Event: payment.paid
X-Xloyal-Timestamp: 1723939200
X-Xloyal-Signature: sha256=<hex_digest>
```

The receiver rejects timestamps outside the configured replay window and verifies the signature against the exact raw bytes before JSON parsing. Xloyal retries failed deliveries with bounded exponential backoff and keeps the payment state unchanged.

## Theme and Browser Security

Themes are declarative versioned JSON. They cannot contain executable JavaScript, arbitrary HTML, arbitrary iframes, or server-side expressions. The hosted page renders through an allow-listed component/template registry.

## Threat Summary

| Threat | Impact | Mitigation |
| --- | --- | --- |
| Token guessing | Unauthorized checkout access | High-entropy token, hash storage, rate limiting |
| Amount tampering | Incorrect payment display/order fulfillment | Server snapshot only |
| Status tampering | False paid order | Backend transition only |
| Open redirect | Phishing or token leakage | Exact tenant allow-list |
| SSE leakage | Payment data disclosure | Token auth, public-safe DTO, connection limits |
| Webhook replay | Duplicate fulfillment | Timestamp window, event ID, receiver idempotency |
| Duplicate payment transition | Double fulfillment | Conditional atomic update and unique event ID |
| Theme injection | XSS/RCE | Declarative schema and component allow-list |
| Tenant crossover | Data disclosure | Tenant/session ownership checks |

## Open Decisions for Phase 2

- The current tenant model has one `webhook_url`. Decide whether to reuse it with a secret relation or add a dedicated webhook endpoint table.
- Confirm the replay window and per-tenant webhook secret rotation policy before delivery is enabled.
- Enumerate the public-safe tenant payload explicitly; no existing admin DTO may be reused without review.
