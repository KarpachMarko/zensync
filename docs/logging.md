# Logging and diagnostics

ZenSync writes structured JSON logs to standard output. The default level is `INFO`. Every API call emits a completion event, including successful reads, status checks and polling.

```json
{"level":"INFO","msg":"API request completed","request_id":"example-request-id","auth":"api_key","user_id":"owner-id","api_key_id":"key-id","method":"GET","path":"/api/v1/system/status","status":200,"response_bytes":287,"duration_ms":1}
```

The response includes the matching `X-Request-Id`. Use it to correlate HTTP completion with integration calls and database errors. `api_key_id` identifies the stored key and is not its secret token. The event also reports client cancellation, write failures and the reason for rejected requests. HTTP 4xx and write failures use WARN; HTTP 5xx and handler panics use ERROR. Handler panics remain available to the HTTP server’s recovery mechanism.

Routine `/healthz` and `/readyz` successes use DEBUG, keeping container probes out of the default access log. They are not API routes. All `/api/` requests are logged at INFO or above, including successful polling.

## Configure verbosity

Set `ZENSYNC_LOG_LEVEL` to `debug`, `info`, `warn` or `error`. Invalid levels produce a warning and fall back to INFO. WARN and ERROR intentionally suppress successful access events.

With the repository’s Compose configuration:

```sh
ZENSYNC_LOG_LEVEL=debug docker compose up -d

docker compose logs -f --tail=100 zensync
```

Changing the environment requires recreating/restarting the application container with the updated configuration. When running the binary directly, pass `ZENSYNC_LOG_LEVEL=debug` in its environment. No separate log file or debug build is required.

DEBUG adds request-start events, successful Deezer/Navidrome/Lossless.am HTTP calls, endpoint timing and health probes. Failed upstream requests are visible at WARN without enabling DEBUG. They report HTTP status or a safe failure category (`dns`, `tls_certificate`, `timeout`, `connection`, `cancelled`, `transport`), never raw transport error URLs. Provider payload/operation rejections include their service and code where available.

The application also logs startup, database initialization, shutdown, background-worker lifecycle, recovery counts/errors, migration task start/completion/failure, acquisition attempts and state transitions, successful sign-ins and key creation/revocation. Task and job IDs associate background integration activity with its durable work record.

Authorization headers, cookies, passwords, provider tokens, query strings, and request/response bodies are excluded. Key labels and user-entered search terms are not included in access logs. Long path identifiers are replaced with `:id`.

## Wavio connection checks

A successful verification should produce a `GET /api/v1/system/status` event with status 200. Wavio accepts an envelope with `success: true` and the status payload in `data`; it reads `data.services.soulseek` to display downloader readiness. ZenSync maps that field to its configured Deezer acquisition readiness.

If that access event is absent, inspect the client and reverse proxy: DNS, TLS, network reachability or proxy rejection can fail before an HTTP request reaches ZenSync. A status 200 proves receipt of that particular request; confirm it is the request from Wavio rather than a separate desktop test. Application logs cannot record connections that never reach the HTTP handler.
