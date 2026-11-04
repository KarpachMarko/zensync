# Media providers

Search has a provider selector saved in the URL (`provider=lossless` or the default Deezer). Each song request has a provider identity; Deezer targets retain their existing `deezer:track:<id>` format. Lossless.am uses `lossless:track:<source>:<id>`, including the source returned by the service. IDs from different catalogs cannot fulfill each other's requests.

The `mediaProvider` interface in `internal/app/providers.go` covers search, recording revalidation, readiness and acquisition. Approval, quality snapshots, library reuse, validation, placement and Navidrome publication are shared. Deezer's album API remains Deezer-specific. Import review and automatic matching search configured music providers. Deezer and lossless.am candidates retain independent source identities in review, ranking and requests. New adapters must return normalized `Track` metadata with a stable provider identity and implement these operations.

## Lossless.am (experimental)

Implemented from the supplied download HAR:

- `POST /api/search` with a JSON `query`.
- `GET /api/download/track?id=…&source=…`, returning FLAC.
- Individual song search and requests, including existing approval and retry/review flows.

Choose **lossless.am** in Search. Enable FLAC in your quality preferences before making a request. The captured API provides no quality-selection contract; this adapter never transcodes or silently substitutes MP3. Album acquisition and previews are unavailable. Import review searches lossless.am when configured, accepts direct `https://lossless.am/app/track/<source>/<id>` links, and supports both individual and bulk acquisition. Automatic matching uses the same conservative metadata checks across configured providers.

An administrator can enter their lossless.am email/password under **Settings → Music providers → Lossless.am**. Once both fields are valid, ZenSync signs in with `POST /api/auth/login`, verifies `/api/me`, and saves the returned session cookies in the encrypted credential store. The password is used only for sign-in and is never stored. A failed login leaves the previous connection intact.

The observed login response sets a 24-hour `jwt_access` cookie at `/` and a 90-day `jwt_refresh` cookie at `/api/auth`. ZenSync respects cookie paths and expiry, so refresh credentials are not sent to search or download endpoints. On HTTP 401 it attempts `POST /api/auth/refresh`, saves rotated cookies and retries the original request once. Device-bound refresh JWTs supply the `device_id` hint observed in the site's client code; login tokens without a device ID use an empty JSON object. Refreshes and connection replacement are serialized to avoid overwriting a newer session. Once refresh expires or is revoked, sign in again; ZenSync does not retain a password for automatic re-login.

The settings UI uses email/password sign-in only. Existing manually configured cookie sessions continue to work and can renew access when they include a refresh cookie. HTTP failures log the endpoint, status and timing without exposing credentials or response bodies. Sign-in errors distinguish login from account verification, and HTTP 451 points to provider or network access restrictions on the ZenSync server. Redirects are rejected to avoid forwarding sessions to another host.

The captured client establishes `GET /api/track/<id>?source=<source>` for exact lookup. Pasted links and worker revalidation require the returned source/ID to match the selected recording. Execution also compares current metadata against the approved snapshot. Downloads use bounded staging (1 GiB), FLAC MIME checks, codec/tag/ISRC/duration verification and full decoding before placement. Lossless.am does not have Deezer's documented `SOURCEID` tag contract, so its files are checked using the selected endpoint identity and recording metadata. Deezer retains its mandatory source-ID check. A duration discrepancy uses the existing explicit retained-audio review.

Validation uses a local API fixture and generated FLAC audio, including password sign-in, cookie paths/expiry, refresh rotation and concurrent requests. The login request and Set-Cookie contract were supplied separately; the refresh request was found in the captured client code. Live requests with Go’s default user agent returned HTTP 451 on November 4, 2026. Repeating login (with deliberately invalid credentials) and account lookup using a browser-compatible user agent returned the expected HTTP 401. The adapter now uses that user agent; live password sign-in, token renewal, a real authenticated download and the service's actual output tags remain unverified. Files whose tags do not match the selected recording fail validation and are not published.
