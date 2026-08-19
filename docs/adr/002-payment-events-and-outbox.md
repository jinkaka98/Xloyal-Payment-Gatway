# ADR 002: Payment Events and Transactional Outbox

Status: Accepted for Phase 2 design

## Decision

Payment events are created only after a domain transition has been validated and persisted. The transition and its event/outbox records are committed in one PostgreSQL transaction.

```text
Payment transition
  -> update invoice/session
  -> insert immutable payment_event
  -> insert outbox_event
  -> commit
  -> outbox worker dispatches SSE and webhook work
```

The outbox is the durable source for pending dispatch. In-memory subscriber registries are transport optimizations only.

## Event Contract

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

Required fields:

```json
{
  "event_id": "evt_opaque_random",
  "tenant_id": "tenant_opaque",
  "invoice_id": "inv_opaque",
  "payment_session_id": "sess_opaque",
  "event_type": "payment.paid",
  "occurred_at": "2026-08-18T00:00:00Z",
  "payload": {}
}
```

`payment_event` rows are immutable. Event IDs are unique. Payloads contain public integration data only and never API keys, provider credentials, browser cookies, or secrets.

## Crash Behavior

If the process crashes after commit but before dispatch, the outbox row remains pending and is retried by the outbox worker. If dispatch succeeds and the process crashes before acknowledgement, delivery is retried using the same event ID; consumers must be idempotent.

Payment state is never rolled back because a webhook or SSE delivery fails.

## Ordering and Idempotency

Ordering is per payment/session, using persisted occurrence sequence or occurrence time plus event ID. Consumers deduplicate by `event_id`. Webhook delivery deduplicates by `(tenant_id, event_id)`.

## Consequences

- Worker and provider transitions must use the same transactional event boundary.
- Existing reconciliation remains responsible for deciding payment status.
- SSE and webhook dispatch can be retried independently.

## Open Decisions for Phase 2

- Decide whether outbox dispatch is a new mode in the existing worker binary or a separate process. Reusing the worker is the default compatibility option.
- Decide whether strict per-payment ordering requires a persisted sequence number in addition to `occurred_at` and `event_id`.
- Decide whether SSE fan-out uses an in-process registry or a future shared transport. Reconnect must always use the snapshot endpoint either way.
