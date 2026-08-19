# ADR 004: Custom Payment Theme

Status: Accepted for Phase 2 design

## Decision

Introduce `PaymentTheme` as a draft/published configuration and `PaymentThemeVersion` as an immutable published snapshot.

Theme status:

```text
DRAFT -> PUBLISHED -> ARCHIVED
```

Publishing creates a new monotonically increasing version. A payment session stores the resolved theme version at creation time. Editing a draft cannot change an active checkout. New sessions use the tenant's latest published default; if none exists, the system default is used.

## Declarative Configuration

The configuration includes a schema version, template key, branding, color tokens, layout tokens, payment visibility flags, timer settings, success copy, and redirect delay. Values are validated by type, range, URL policy, and an allow-list of template keys.

The minimum template keys are `modern`, `minimal`, `dark`, `corporate`, and `compact`. They share one renderer and component system; templates do not fork payment page source code.

## Admin Lifecycle

Admin operations are draft-oriented:

```text
create draft -> edit -> preview -> publish -> optional unpublish/archive
```

Delete is blocked when the theme is the tenant's active default unless a replacement is selected. Preview and production use the same rendering component with different data/loading contexts.

## Security Constraints

No custom JavaScript, arbitrary HTML execution, arbitrary iframe, CSS expression, server-side template expression, or remote executable asset is accepted. Logo and asset URLs use an explicit safe URL policy.

## Open Decisions for Phase 2

- Lock the exact JSON schema and maximum serialized size before accepting admin input.
- Decide whether logos/assets must be tenant-hosted HTTPS resources or use a server-managed asset store.
- Seed the system default theme through a forward-compatible migration rather than application memory.
