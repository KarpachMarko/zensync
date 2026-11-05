# Song deletion requests

The database upgrades automatically to schema version 5 on startup; older ZenSync binaries will reject this database. Back up the database and data directory before upgrading.

All signed-in users can request removal of a song from the shared library. Administrators and members with the separate **Allow deleting music without approval** grant receive automatic approval. Direct acquisition permission does not grant deletion permission. Administrators grant/revoke deletion permission in Settings → Users; members cannot approve requests or change grants. Other users' requests appear in the administrator's Requests page. Owners can withdraw their own pending requests; administrators approve or decline them there.

Approved deletion runs as a durable task after active acquisitions finish. “Without approval” means automatically queued, rather than waiting for an administrator. Library → Delete song / Request deletion submits the same request as the API. Requests shows decision and execution status; Activity supports retrying failed deletion tasks.

ZenSync verifies the original audio against Navidrome and snapshots its path and SHA-256 before accepting a request. A changed or unsafe file fails removal. Both services must see the same music directory. For files acquired outside ZenSync, enable `Subsonic.DefaultReportRealPath = true` in Navidrome if its virtual paths do not identify the real files; different container mount prefixes are supported. The shared Navidrome connection needs original-file download and scan permission. Ambiguous paths are rejected.

## External client authentication

Create a key in **Settings → API keys**. Send `Authorization: Bearer <token>` over HTTPS (or your trusted local connection). The key is shown once, stored only as a hash, belongs to its creator, and can be revoked immediately. Select the `deletion:request` scope to submit deletion requests and poll their own status, including when the owner is an administrator. The same key can request downloads or read music when you select those additional scopes. It cannot approve requests, administer users, or access other users' requests. Permission checks use the owner's current deletion grant.

A client such as Wavio needs to implement these calls; its existing wishlist removal is unrelated to file deletion. These are native ZenSync `/api/v2` endpoints, not the SoulSync compatibility API. Bearer calls have no cookie, Origin or CSRF requirement. Cross-origin browser CORS support is not enabled; native clients and server integrations can call directly.

### Submit

`POST /api/v2/deletion-requests`

Headers: `Authorization: Bearer <token>`, `Content-Type: application/json`, and an `Idempotency-Key` of 8–128 characters (excluding the reserved `album:` prefix).

```json
{"songId":"<Navidrome song ID>"}
```

Returns HTTP 201 with `{"id":"<request ID>"}`. Repeating the same owner/key/song returns HTTP 200 and the original ID, including after removal. Reusing a key for a different target returns 409. Another pending removal for the same file also returns 409. Client-supplied filesystem paths are never accepted. HTTP 400 means invalid input or unavailable song, 401 invalid/revoked key, and 409 an unsafe, changed, ambiguous, or unresolvable file/conflict.

### Poll

`GET /api/v2/deletion-requests/<request ID>` with the same Bearer key.

```json
{
  "id":"<request ID>",
  "decision":"approved",
  "reason":"Deletion allowed without approval",
  "state":"completed",
  "detail":"Removed from shared library; recovery copy retained"
}
```

`decision` is `awaiting_approval`, `approved`, `declined`, or `withdrawn`. `state` is empty before approval, then `queued`, `blocked`, `failed`, or `completed`. Approval alone does not mean the file has been removed. Poll until completed/failed; show the reason/detail for rejected or failed work. A revoked direct-delete grant returns unstarted removal to administrator approval. A successful response never includes the recovery path or another user's request data.

## Session API for administration

The existing `/api/v2/session` login establishes an HttpOnly session cookie and returns a CSRF token. Mutating session calls require a matching `Origin` and `X-CSRF-Token` header. Administration requires a browser session. A unified API key with `requests:read` can also list its owner’s requests:

- `GET /api/v2/requests`: own requests, or all requests for admins. Deletions have `kind: "delete"`, `target: "navidrome:song:<id>"`, and `state`/`detail` after approval.
- `POST /api/v2/requests/<id>`: `{"decision":"approved","reason":""}` or `declined` with a required reason (admin only); `withdrawn` (pending request owner only).
- `PUT /api/v2/users/<user ID>/permissions`: `{"directDelete":true}` (admin only). Set false to revoke. This does not change acquisition permission.
- `POST /api/v2/users`: optional `directDelete` boolean when creating a member.
- `POST /api/v2/tasks/<job ID>`: retry a failed task (owner/admin); automatic deletion permission is rechecked before file removal.
- Manage unified keys through `/api/v2/api-keys`; select `deletion:request` alongside any other needed scopes. See [API keys](api-keys.md). The former `/deletion-api-keys` routes remain aliases over the same keys. Existing keys keep their tokens and deletion-only access.

## Removal and recovery

Deletion affects every user of the shared library. ZenSync retains the original bytes at `<ZenSync data directory>/trash/<request ID>/audio`, with the source path and checksum in the `song_deletions` database journal. Recovery storage must be outside the scanned music directory and have space for a complete copy; no original is removed until the recovery copy is persisted and verified. Copies are retained until manually cleaned up. Back up the ZenSync database and data directory together.

After removing a file, ZenSync clears shared availability and migration matches, defers affected migration songs, cancels old acquisition jobs for the asset, hides the removed ID from its library results, and requests a Navidrome scan. An explicitly approved new acquisition can add the song again. Removing a playlist entry, favorite, or acquisition request never removes audio. Navidrome may retain a missing-file record depending on its purge settings; ZenSync does not edit Navidrome's database or purge playlist/history records. A completed task means the file is removed and the scan request succeeded, not that Navidrome finished scanning.

If scanning fails after removal, the task reports that the song was removed; retry requests the scan again without deleting another file. Interrupted removals reconcile against the verified recovery copy. To restore manually, stop ZenSync, read the original relative path from `song_deletions` for the request, verify the copy's SHA-256, and copy it back to that path without overwriting another file. Clear that request's removed marker (`UPDATE song_deletions SET phase='restored' WHERE request_id='<request ID>'`) and restart/rescan Navidrome. ZenSync has no restore button yet.
