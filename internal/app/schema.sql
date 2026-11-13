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
 state TEXT NOT NULL DEFAULT 'queued', stage TEXT NOT NULL DEFAULT 'Queued for acquisition',
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
CREATE INDEX IF NOT EXISTS requests_owner_target ON requests(owner,target);
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
CREATE TABLE IF NOT EXISTS audio_duration_approvals (
 checksum TEXT NOT NULL, recording TEXT NOT NULL, quality TEXT NOT NULL,
 duration REAL NOT NULL, actor TEXT NOT NULL REFERENCES users(id),
 request_id TEXT NOT NULL REFERENCES requests(id), created TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
 PRIMARY KEY(checksum,recording,quality)
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

CREATE TABLE IF NOT EXISTS acquisition_checks (job_id TEXT PRIMARY KEY REFERENCES jobs(id), checked_at INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS migration_task_wakeups (
 migration_id TEXT NOT NULL REFERENCES migrations(id), action TEXT NOT NULL,
 PRIMARY KEY(migration_id,action)
);

-- Deletion grants are independent of direct acquisition permission.
CREATE TABLE IF NOT EXISTS delete_permissions (
 user_id TEXT PRIMARY KEY REFERENCES users(id)
);
CREATE TABLE IF NOT EXISTS song_deletions (
 request_id TEXT PRIMARY KEY REFERENCES requests(id), song_id TEXT NOT NULL,
 file_path TEXT NOT NULL, checksum TEXT NOT NULL,
 phase TEXT NOT NULL DEFAULT 'pending'
);
CREATE TABLE IF NOT EXISTS deletion_api_keys (
 id TEXT PRIMARY KEY, owner TEXT NOT NULL REFERENCES users(id), token_hash TEXT NOT NULL UNIQUE,
 created TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT OR IGNORE INTO schema_version VALUES(3);

CREATE TABLE IF NOT EXISTS invitations (
 id TEXT PRIMARY KEY, token_hash TEXT NOT NULL UNIQUE, username TEXT NOT NULL UNIQUE,
 inviter TEXT NOT NULL REFERENCES users(id), expires INTEGER NOT NULL,
 created TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT OR IGNORE INTO schema_version VALUES(4);

CREATE TABLE IF NOT EXISTS audio_uploads (
 target TEXT PRIMARY KEY, owner TEXT NOT NULL REFERENCES users(id),
 file_path TEXT NOT NULL, metadata TEXT NOT NULL, quality TEXT NOT NULL,
 checksum TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS migration_acquisition_batches (
 owner TEXT NOT NULL REFERENCES users(id), idem TEXT NOT NULL,
 migration_id TEXT NOT NULL REFERENCES migrations(id) ON DELETE CASCADE,
 fingerprint TEXT NOT NULL, result TEXT NOT NULL,
 PRIMARY KEY(owner,idem)
);

-- Former associations survive recording changes, independently of current links.
CREATE TABLE IF NOT EXISTS request_source_history (
 source_id TEXT NOT NULL REFERENCES source_songs(id) ON DELETE CASCADE,
 request_id TEXT NOT NULL REFERENCES requests(id),
 PRIMARY KEY(source_id,request_id)
);
CREATE INDEX IF NOT EXISTS request_source_history_request ON request_source_history(request_id);

CREATE TABLE IF NOT EXISTS provider_tracks (target TEXT PRIMARY KEY, metadata TEXT NOT NULL);

-- One credential store for native and compatibility integrations. Existing keys
-- retain their original privilege, token hash and identifier.
CREATE TABLE IF NOT EXISTS api_keys (
 id TEXT PRIMARY KEY, owner TEXT NOT NULL REFERENCES users(id),
 token_hash TEXT NOT NULL UNIQUE, label TEXT NOT NULL,
 scopes TEXT NOT NULL, expires INTEGER, created TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT OR IGNORE INTO api_keys(id,owner,token_hash,label,scopes,created)
 SELECT id,owner,token_hash,'Deletion key','["deletion:request"]',created FROM deletion_api_keys;
DROP TABLE deletion_api_keys;
INSERT OR IGNORE INTO schema_version VALUES(5);

CREATE TABLE IF NOT EXISTS compatibility_wishlist (
 id INTEGER PRIMARY KEY AUTOINCREMENT, owner TEXT NOT NULL REFERENCES users(id),
 target TEXT NOT NULL, track_data TEXT NOT NULL, request_id TEXT REFERENCES requests(id),
 created TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP, UNIQUE(owner,target)
);
CREATE TABLE IF NOT EXISTS compatibility_downloads (
 request_id TEXT PRIMARY KEY REFERENCES requests(id), owner TEXT NOT NULL REFERENCES users(id),
 track_data TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS compatibility_albums (
 id INTEGER PRIMARY KEY AUTOINCREMENT, server_id TEXT NOT NULL UNIQUE
);
INSERT OR IGNORE INTO schema_version VALUES(6);

-- Reviewed metadata stays separate from source tags and acquisition identity.
CREATE TABLE IF NOT EXISTS metadata_edits (
 song_id TEXT PRIMARY KEY, metadata TEXT NOT NULL,
 actor TEXT NOT NULL REFERENCES users(id), updated TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT OR IGNORE INTO schema_version VALUES(7);

CREATE TABLE IF NOT EXISTS metadata_jobs (
 id TEXT PRIMARY KEY, actor TEXT NOT NULL REFERENCES users(id), changes TEXT NOT NULL, server_url TEXT NOT NULL,
 state TEXT NOT NULL DEFAULT 'queued', message TEXT NOT NULL DEFAULT 'Queued for tag update',
 done INTEGER NOT NULL DEFAULT 0, created TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS metadata_files (
 job_id TEXT NOT NULL REFERENCES metadata_jobs(id), song_id TEXT NOT NULL,
 file_path TEXT NOT NULL, before_hash TEXT NOT NULL, after_hash TEXT NOT NULL,
 PRIMARY KEY(job_id,song_id)
);
INSERT OR IGNORE INTO schema_version VALUES(8);

CREATE TABLE IF NOT EXISTS metadata_dismissals (
 server_key TEXT NOT NULL, rule_key TEXT NOT NULL,
 actor TEXT NOT NULL REFERENCES users(id), created TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
 PRIMARY KEY(server_key,rule_key)
);
INSERT OR IGNORE INTO schema_version VALUES(9);

-- Keep-song deletions update accessible playlists before removing their source file.
CREATE TABLE IF NOT EXISTS deletion_replacements (
 request_id TEXT PRIMARY KEY REFERENCES song_deletions(request_id),
 keep_song_id TEXT NOT NULL, server_url TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS deletion_playlist_updates (
 request_id TEXT NOT NULL REFERENCES deletion_replacements(request_id),
 account_key TEXT NOT NULL, playlist_id TEXT NOT NULL,
 before_ids TEXT NOT NULL, after_ids TEXT NOT NULL,
 state TEXT NOT NULL DEFAULT 'prepared',
 PRIMARY KEY(request_id,account_key,playlist_id)
);
INSERT OR IGNORE INTO schema_version VALUES(10);

CREATE TABLE IF NOT EXISTS deletion_restores (
 request_id TEXT PRIMARY KEY REFERENCES song_deletions(request_id),
 phase TEXT NOT NULL DEFAULT 'preparing', server_url TEXT NOT NULL
);
INSERT OR IGNORE INTO schema_version VALUES(11);
