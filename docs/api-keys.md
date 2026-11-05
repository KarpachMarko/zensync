# Integration API keys

Create one key per app under Settings → API keys. A key has a required label (1–100 characters), selected scopes, and an optional expiration. It acts as its owner and follows their current download/deletion permissions and approval rules. It cannot approve requests, administer users, change settings, or manage credentials.

Session-authenticated management (mutations require Origin and CSRF):

- `GET /api/v2/api-keys`: list your key metadata. Tokens and hashes are never returned.
- `POST /api/v2/api-keys`: `{ "label": "My music app", "scopes": ["search:read", "requests:create", "requests:read", "deletion:request"], "expires": null }`. `expires` is an optional Unix timestamp in seconds, strictly in the future. Returns metadata and a one-time `token`. Maximum 10 keys per user; revoke expired keys to free slots.
- `DELETE /api/v2/api-keys/<id>`: revoke your key immediately.

Send `Authorization: Bearer <token>` to use a key. Bearer requests need no browser cookie, Origin, or CSRF token. Expired/revoked keys return 401; missing scopes return 403.

| Scope | Allowed native endpoints |
| --- | --- |
| `search:read` | `GET /api/v2/search` |
| `library:read` | `GET /api/v2/library` |
| `requests:read` | `GET /api/v2/requests` (owner’s requests and progress, including for admin-owned keys) |
| `requests:create` | `POST /api/v2/requests` (acquisition requests) |
| `deletion:request` | `POST /api/v2/deletion-requests`, `GET /api/v2/deletion-requests/<id>` (owner’s status) |

A single key may have all these scopes. Deletion is a native ZenSync extension; it does not need a second credential. See [deletion API](deletion-api.md) for request bodies and idempotency requirements.

SoulSync compatibility remains a separate adapter over these services, as described in [the compatibility assessment](assessment.md). This change establishes shared, user-bound Bearer credentials; it does not add `/api/v1` SoulSync endpoints or claim full compatibility. `X-Profile-Id`, when provided, must match the key owner’s ZenSync user ID; another profile is rejected rather than falling back to an administrator. Native apps can use `/api/v2` scopes for additional functionality.

Existing deletion keys migrate automatically, retaining their identifier, token hash, creation time and deletion-only scope, with label “Deletion key” and no expiration. The former `/api/v2/deletion-api-keys` management routes are aliases over the unified store; legacy creation with `{}` still creates a deletion-scoped key.
