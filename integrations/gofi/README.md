# GoFi progress adapter

Docker already builds d-fi from the pinned GoFi 2.3.5 commit. It copies
`zensync_progress.go.txt` as `zensync_progress.go` into `internal/dfi` and changes the CLI's hook factory to
`zenSyncDownloadHooks`. The adapter wraps the existing byte-progress and phase
callbacks; acquisition, authentication, resume, decryption, and tagging remain
upstream. The image retains the upstream MIT license.

With `ZENSYNC_PROGRESS=1`, the adapter writes lines prefixed `ZENSYNC_TRANSFER `
containing JSON with `phase`, `downloaded`, and `total`. Only fixed phases and
numeric counters are emitted. ZenSync discards all other output, validates and
bounds each record, and calculates rate from successive samples. A resumed
transfer's initial bytes are a baseline, not newly transferred bytes. Phase
changes clear transfer metrics. A stock d-fi binary still works, with an animated
indeterminate bar because it does not emit these records.

GoFi's CLI pipeline is internal and its session configuration is process-global.
Keeping the source-built adapter in a subprocess preserves independent credentials,
parallel jobs, timeouts, and cancellation without copying the downloader or its
protocol into ZenSync.
