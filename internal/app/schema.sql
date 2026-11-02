CREATE TABLE IF NOT EXISTS schema_version(version INTEGER PRIMARY KEY);
INSERT OR IGNORE INTO schema_version VALUES(1);
CREATE TABLE IF NOT EXISTS users (
 id TEXT PRIMARY KEY, username TEXT NOT NULL UNIQUE, password BLOB NOT NULL,
 role TEXT NOT NULL CHECK(role IN ('admin','member')), direct INTEGER NOT NULL DEFAULT 0,
 quality TEXT NOT NULL DEFAULT '["lossless","320","128"]', created TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS sessions (
 token TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(id), csrf TEXT NOT NULL, expires INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS requests (
 id TEXT PRIMARY KEY, owner TEXT NOT NULL REFERENCES users(id), idem TEXT NOT NULL,
 target TEXT NOT NULL, title TEXT NOT NULL, artist TEXT NOT NULL, upgrade INTEGER NOT NULL,
 quality TEXT NOT NULL, decision TEXT NOT NULL, reason TEXT NOT NULL DEFAULT '',
 actor TEXT REFERENCES users(id), created TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
 UNIQUE(owner,idem)
);
CREATE TABLE IF NOT EXISTS jobs (
 id TEXT PRIMARY KEY, request_id TEXT NOT NULL UNIQUE REFERENCES requests(id),
 state TEXT NOT NULL DEFAULT 'blocked', stage TEXT NOT NULL DEFAULT 'Integration not configured',
 created TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS attempts (
 id TEXT PRIMARY KEY, job_id TEXT NOT NULL REFERENCES jobs(id), stage TEXT NOT NULL,
 outcome TEXT NOT NULL, created TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS dependencies (
 job_id TEXT NOT NULL REFERENCES jobs(id), depends_on TEXT NOT NULL REFERENCES jobs(id), PRIMARY KEY(job_id,depends_on)
);
CREATE TABLE IF NOT EXISTS events (
 id INTEGER PRIMARY KEY AUTOINCREMENT, owner TEXT NOT NULL REFERENCES users(id),
 actor TEXT NOT NULL REFERENCES users(id), kind TEXT NOT NULL, resource TEXT NOT NULL,
 created TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS requests_owner ON requests(owner,created);
CREATE INDEX IF NOT EXISTS events_owner ON events(owner,id);
CREATE TABLE IF NOT EXISTS secrets (name TEXT PRIMARY KEY, value BLOB NOT NULL);
CREATE TABLE IF NOT EXISTS settings (name TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS migrations (
 id TEXT PRIMARY KEY, owner TEXT NOT NULL REFERENCES users(id), fingerprint TEXT NOT NULL,
 name TEXT NOT NULL, state TEXT NOT NULL DEFAULT 'staged', source TEXT NOT NULL, report TEXT NOT NULL,
 favorites INTEGER NOT NULL DEFAULT 0, target_user TEXT NOT NULL DEFAULT '', error TEXT NOT NULL DEFAULT '',
 created TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP, UNIQUE(owner,fingerprint)
);
CREATE TABLE IF NOT EXISTS source_songs (
 id TEXT PRIMARY KEY, migration_id TEXT NOT NULL REFERENCES migrations(id), source_id TEXT NOT NULL,
 metadata TEXT NOT NULL, server_id TEXT NOT NULL DEFAULT '', candidate TEXT NOT NULL DEFAULT '',
 match_state TEXT NOT NULL DEFAULT 'unresolved', deferred INTEGER NOT NULL DEFAULT 0,
 favorite_state TEXT NOT NULL DEFAULT 'pending', UNIQUE(migration_id,source_id)
);
CREATE TABLE IF NOT EXISTS migration_playlists (
 id TEXT PRIMARY KEY, migration_id TEXT NOT NULL REFERENCES migrations(id), name TEXT NOT NULL,
 metadata TEXT NOT NULL, entries TEXT NOT NULL, selected INTEGER NOT NULL,
 state TEXT NOT NULL DEFAULT 'active', remote_id TEXT NOT NULL DEFAULT '',
 baseline TEXT NOT NULL DEFAULT '', desired TEXT NOT NULL DEFAULT '', error TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS migration_tasks (
 id TEXT PRIMARY KEY, owner TEXT NOT NULL REFERENCES users(id), migration_id TEXT NOT NULL REFERENCES migrations(id),
 action TEXT NOT NULL, state TEXT NOT NULL DEFAULT 'queued', message TEXT NOT NULL DEFAULT '',
 created TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS migration_task_active ON migration_tasks(migration_id,action) WHERE state IN ('queued','running');
CREATE TABLE IF NOT EXISTS request_sources (
 source_id TEXT NOT NULL REFERENCES source_songs(id), request_id TEXT NOT NULL REFERENCES requests(id),
 PRIMARY KEY(source_id,request_id)
);
CREATE TABLE IF NOT EXISTS job_runtime (
 job_id TEXT PRIMARY KEY REFERENCES jobs(id), metadata TEXT NOT NULL DEFAULT '',
 file_path TEXT NOT NULL DEFAULT '', quality TEXT NOT NULL DEFAULT '', server_id TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS assets (
 target TEXT PRIMARY KEY, file_path TEXT NOT NULL, quality TEXT NOT NULL, metadata TEXT NOT NULL,
 server_id TEXT NOT NULL DEFAULT ''
);
INSERT OR IGNORE INTO schema_version VALUES(2);

CREATE TABLE IF NOT EXISTS request_metadata (request_id TEXT PRIMARY KEY REFERENCES requests(id), metadata TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS scan_schedule (job_id TEXT PRIMARY KEY REFERENCES jobs(id), checked_at INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS source_resolutions (source_id TEXT PRIMARY KEY REFERENCES source_songs(id), revision TEXT NOT NULL);

CREATE TABLE IF NOT EXISTS scan_waits (job_id TEXT PRIMARY KEY REFERENCES jobs(id), started_at INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS asset_relocations (
 id TEXT PRIMARY KEY, target TEXT NOT NULL UNIQUE REFERENCES assets(target),
 owner TEXT NOT NULL REFERENCES users(id), old_path TEXT NOT NULL, new_path TEXT NOT NULL DEFAULT '',
 checksum TEXT NOT NULL DEFAULT '', server_id TEXT NOT NULL DEFAULT '',
 state TEXT NOT NULL DEFAULT 'queued', resume_state TEXT NOT NULL DEFAULT 'queued',
 message TEXT NOT NULL DEFAULT 'Organizing legacy download', checked_at INTEGER NOT NULL DEFAULT 0,
 started_at INTEGER NOT NULL DEFAULT 0, created TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS album_requests (request_id TEXT PRIMARY KEY REFERENCES requests(id), snapshot TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS request_children (
 parent_id TEXT NOT NULL REFERENCES requests(id), child_id TEXT NOT NULL UNIQUE REFERENCES requests(id),
 position INTEGER NOT NULL, PRIMARY KEY(parent_id,position)
);
