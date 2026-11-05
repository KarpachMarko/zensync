# Wavio / SoulSync compatibility

In Wavio, select the **SoulSync** integration. Enter the ZenSync server root (for example `https://zensync.example.com`, without `/api/v1`) and one API key created in Settings → API keys. Keep the default profile ID **1**. No second credential is needed for native ZenSync actions.

Keep the default scopes: `search:read`, `library:read`, `requests:read`, `requests:create`. Deletion is an optional native extension requiring `deletion:request`. Existing migrated deletion-only keys need replacing with a key that includes the integration scopes to search or acquire music.

The adapter currently uses Deezer metadata and ZenSync’s Deezer acquisition backend. Configure the shared Navidrome connection, Deezer credentials, music mount and d-fi for downloads. `services.soulseek` is the legacy readiness field Wavio reads; ZenSync maps it to configured Deezer acquisition readiness, not an installed Soulseek service. `spotify` and `hydrabase` are false. A reachable server can authenticate successfully while download readiness is false.

All `/api/v1` responses use `{success,data,error,pagination}`. Errors include `{code,message}`. Authentication requires a Bearer key; cookie sessions do not authorize compatibility routes. Expiration, revocation and scopes use the same store as `/api/v2`. Profile 1 is an alias for the key owner’s default profile, never another ZenSync user. An explicit owner ID also works; other profiles return 403.

| Endpoint | Scope | Behavior |
| --- | --- | --- |
| `GET /system/status` | Any valid key | Uptime, backend readiness and capabilities |
| `POST /search/tracks` | `search:read` | Deezer tracks, flat SoulSync fields and round-trip provider-qualified IDs |
| `POST /search/artists` | `search:read` | Deezer artists and source |
| `GET /wishlist` | `requests:read` | Owner’s durable acquisition intents; failed requests stay visible |
| `POST /wishlist` | `requests:create` | Store Wavio’s `track_data` payload after provider ID verification |
| `DELETE /wishlist/<track ID>` | `requests:create` | Remove only the owner’s intent; ongoing tasks and files are unchanged |
| `POST /wishlist/process` | `requests:create` | Process selected `track_ids` (1–50), respecting account approval and quality |
| `GET /downloads` | `requests:read` | Owner’s persistent per-track queue and terminal history, limit/offset pagination |
| `GET /library/recently-added?type=albums` | `library:read` | Newest shared Navidrome albums with usable Navidrome cover paths |

Paths in this table are relative to `/api/v1`. The request and response contract follows [Wavio’s SoulSync services](https://github.com/Joel-Mercier/wavio/tree/85f3e7f4c188a6df9f41c40c4c9744b7337de21f/apps/mobile/services/soulsync).

Adding an intent does not start a download. Processing creates ordinary ZenSync requests, with member approval required unless granted otherwise. Repeated processing does not duplicate requests. Awaiting approval is `pending`; Navidrome indexing is `post_processing`. Only verified `available`/completed work reports `completed` with progress 100 and disappears from the wishlist. Failed/blocked requests retain their details. Removing an intent does not erase download history. Downloads and wishlist state survive application restarts through SQLite.

Artist following/watchlist scans are not implemented yet and return an explicit `501 NOT_AVAILABLE`, including for `/watchlist` reads. The status response advertises `watchlist: false`. This supports Wavio’s connection/search/track acquisition flow; it does not claim complete SoulSync API parity or artist automation.

## Diagnostics

Every API call, including a successful `/api/v1/system/status`, emits a structured completion log to standard output. Logs include `request_id`, method, path, status, response bytes, elapsed milliseconds and authentication type; authenticated requests also include `user_id` and `api_key_id` (identifiers, never tokens). The same request ID is returned as `X-Request-Id`. Successful requests use INFO, client errors WARN, and server errors ERROR. Authorization headers, cookies, query strings and request/response bodies are excluded; long path identifiers are redacted. See [logging configuration](logging.md) for debug diagnostics.

For Docker, inspect `docker compose logs --tail=100 zensync` after attempting a connection (or `docker logs <container>` if running without Compose). A bad key yields 401, a missing scope or forbidden profile yields 403, and an unsupported compatibility route yields a structured 404/501. CI retains failing Playwright traces and screenshots as the `playwright-failures` artifact.
