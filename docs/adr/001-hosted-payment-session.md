# ADR 001: Hosted Payment Session

Status: Accepted for Phase 2 design

## Context

The repository currently treats `Invoice` as the financial request and uses provider or browser reconciliation to move it through `creating`, `pending`, `paid`, `expired`, and `failed`. The web application is an admin console; it is not a customer checkout surface.

A hosted checkout needs browser state, a public URL, expiry, cancellation, redirect lifecycle, and theme resolution. Those concerns must not change the financial meaning of an invoice.

## Decision

Introduce `PaymentSession` as a separate checkout aggregate layered on top of an existing invoice.

```text
Tenant -> Invoice -> PaymentSession -> PaymentEvent
```

Responsibilities:

- `Invoice`: amount, currency, provider reference, QR/payment payload, financial status, and reconciliation truth.
- `PaymentSession`: public checkout token, browser lifecycle, resolved theme version, redirect destinations, and checkout expiry.
- `PaymentEvent`: immutable persisted history of domain transitions.
- `WebhookDelivery`: delivery attempt state for an event, not payment state.
- `PaymentTheme`: declarative presentation configuration.

The backend remains the only authority for payment status. Browser state, query parameters, SSE messages, and redirect parameters never change an invoice.

## Invoice State Contract

Existing invoice transitions remain:

```text
creating -> pending
creating -> failed
pending  -> paid
pending  -> expired
pending  -> failed
```

`paid`, `expired`, and `failed` are terminal. `cancelled` is not added to the invoice state machine for browser cancellation. A cancelled checkout session may still reference an invoice that is later reconciled according to the existing financial rules.

## PaymentSession State Contract

Valid transitions:

```text
OPEN             -> PAYMENT_PENDING
PAYMENT_PENDING  -> PAID
PAYMENT_PENDING  -> CANCELLED
PAYMENT_PENDING  -> EXPIRED
PAYMENT_PENDING  -> FAILED
PAID             -> REDIRECTING
REDIRECTING      -> CLOSED
```

Invalid transitions include `PAID -> PAYMENT_PENDING`, `PAID -> CANCELLED`, `PAID -> EXPIRED`, `CANCELLED -> PAID`, `EXPIRED -> PAYMENT_PENDING`, and `FAILED -> PAID`. Every transition must be checked and persisted atomically.

## Public URL

The hosted route is `/pay/{public_token}`. The token is random and opaque; it must not encode invoice ID, tenant ID, amount, timestamp, order ID, or provider reference.

## Consequences

- Existing invoice/provider/worker logic remains the payment engine.
- A session can be cancelled without rewriting invoice financial state.
- A session stores an immutable resolved theme version, so later theme edits do not change an active checkout.
- Phase 2 must add repository/service boundaries without duplicating payment reconciliation.

## Non-goals

This ADR does not add tables, routes, UI, SSE, webhook delivery, or theme implementation.

## Open Decisions for Phase 2

- Confirm whether session creation accepts only an existing `pending` invoice or also a provider-creation `creating` phase. The existing API returns after provider creation, so pending-only is the lower-risk first implementation.
- Session expiry must be no later than linked invoice expiry. The exact override policy remains to be encoded in the API implementation.
- SHA-256 token lookup is selected for the contract; key rotation and keyed-digest requirements should be confirmed before migration.
- `payment_id` is an opaque public payment identifier. Its exact relationship to the existing invoice ID must be fixed before generated clients are published.
