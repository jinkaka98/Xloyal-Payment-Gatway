# Hosted Payment Page

The public checkout route is `/pay/{public_token}`. It does not use an admin
cookie, API key, provider credential, or webhook secret.

## Customer Flow

1. The browser requests the public payment snapshot.
2. The page renders the server amount, QR payload, expiry, and immutable theme
   configuration.
3. Only after a successful snapshot does it open SSE using `after_sequence`.
4. Events are deduplicated by sequence. A sequence gap triggers a new snapshot
   before further state is trusted.
5. Terminal states are derived from the snapshot or persisted payment event.

The browser does not create a payment, regenerate a QR code, or validate
payment with a provider. Multiple tabs can safely view the same token.

## Connection Lifecycle

The page computes expiry from `expires_at` and `server_now`; it does not use a
decrement-only timer. When SSE disconnects it displays a connection indicator,
refreshes the snapshot, then reconnects with bounded exponential backoff. A
network failure never changes the payment state to failed.

## Cancel and Redirect

The cancel button calls the public cancel endpoint and waits for its response.
For a cancel-versus-paid race the response snapshot is authoritative. The page
uses only server-provided, allow-listed redirect URLs. A success redirect uses
the theme-configured delay (default five seconds); terminal fallback pages keep
a `Kembali ke merchant` button.

## Security

All remote text is rendered with normal React escaping. The page has no
`dangerouslySetInnerHTML` use and has no path to tenant secrets. Theme logo
URLs, text, colors, and layout are treated as presentation data only.
