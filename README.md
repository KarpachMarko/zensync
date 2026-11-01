# ZenSync

A self-hosted music library companion for Navidrome, focused on collecting music and keeping personal playlists in sync.

## Current status

Planning and interactive UI prototypes. The Go backend and production authentication, downloads and synchronization have not been implemented. Mockups use sample data and simulated operations.

## Product scope

- Go + SQLite backend and a compact React/TypeScript interface.
- Apple/iTunes and Deezer metadata; Deezer downloads through the existing d-fi Go binary.
- Complete Apple Music XML catalog and playlist import, public Apple playlist links and playlist files.
- Navidrome compatibility and SoulSync API compatibility targeting Wavio first.
- Shared base music library with individual playlists, favorites, imports and followed artists.
- Admin-approved acquisition requests by default, with an optional per-member direct-acquisition grant. Members cannot delete song files.
- Dedicated pages, collapsible sidebar, compact music rows, server Activity and persistent unresolved-song placeholders with Skip for now.
- No recommendations, movies, audiobooks, podcasts or playback engine in the initial scope.

## Start here

- [Implementation plan](docs/implementation-plan.md)
- [Multi-user permissions and request workflow](docs/multi-user-design.md)
- [Original source investigation and preliminary assessment](docs/assessment.md)
- [Latest five compact detail mockups](mockups/details/index.html)
- [Mockup instructions and validation](mockups/details/README.md)

Open the mockup HTML directly; no server or build step is required. Use Preview as to compare sample admin/member behavior. The earlier sets remain under `mockups/revised/` and `mockups/` for design history.

The assessment's original single-user estimate predates the expanded requirements and is not the current implementation commitment.
