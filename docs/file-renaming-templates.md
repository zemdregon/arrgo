# File Renaming & Folder Organization Templates

## Overview
arrgo now ships with a unified renaming engine that keeps television, movie,
and music libraries consistently organized. Operators can configure independent
folder/file templates for each catalog type, preview the impact of changes, and
run history-aware rename jobs that avoid clobbering previously curated paths.
The implementation lives in [`internal/library/renamer.go`](../internal/library/renamer.go)
and is exercised by [`internal/library/renamer_test.go`](../internal/library/renamer_test.go).

Key capabilities include:

- **Configurable templates** for episodic, cinematic, and music collections with
  sensible defaults inspired by Sonarr/Radarr/Lidarr conventions.
- **Tokenized patterns** that support zero-padded numbers, case conversion, and
  automatic cleanup of illegal filesystem characters.
- **History-aware rename jobs** that record every execution, reuse the most
  recent target paths, and expose prior job IDs/timestamps for auditing.
- **Dry-run previews** that show planned operations without mutating the
  library, enabling safe iteration before enqueuing a real job.
- **Conflict detection** that flags duplicate target paths so collisions can be
  resolved prior to dispatching workers.

## Template Sets
Templates are grouped per media family and each group contains the folder/file
patterns required to produce a complete relative path:

| Media type | Folder hierarchy | File pattern |
| --- | --- | --- |
| **Episodes** | Series folder → Season folder | Episode filename |
| **Movies** | Movie folder | Movie filename |
| **Music** | Artist folder → Album folder | Track filename |

The built-in defaults are:

```text
Episodes:
  Series: {SeriesTitle}
  Season: Season {SeasonNumber:00}
  File:   {SeriesTitle} - S{SeasonNumber:00}E{EpisodeNumber:00} - {EpisodeTitle} [{Quality}].{Extension:lower}
Movies:
  Folder: {MovieTitle} ({ReleaseYear})
  File:   {MovieTitle} ({ReleaseYear}) [{Quality}].{Extension:lower}
Music:
  Artist: {ArtistName}
  Album:  {AlbumTitle} ({ReleaseYear})
  File:   {TrackNumber:00} - {TrackTitle}.{Extension:lower}
```

Admins can override any subset of the patterns by calling
`ConfigureRenameTemplates`. The service fills in omitted values with the
defaults so partial updates remain safe.

## Token Reference
Templates interpolate metadata tokens surrounded by braces. Tokens support an
optional format suffix after a colon.

| Token | Description | Applicable templates |
| --- | --- | --- |
| `SeriesTitle` | Series display title (falls back to entity title) | Episode folders/files |
| `SeasonNumber` | Season ordinal | Episode season/episode |
| `EpisodeNumber` | Season-relative episode number | Episode files |
| `AbsoluteEpisodeNumber` | Absolute numbering for anime/scene releases | Episode files |
| `EpisodeTitle` | Episode title | Episode files |
| `MovieTitle` | Movie title | Movie folders/files |
| `ReleaseYear` | Release year when available | All |
| `Quality` | Quality/edition string (e.g., `WEBDL-1080p`) | All |
| `Resolution` | Video or audio resolution | All |
| `Edition` | Edition/cut label | All |
| `Source` | Source type (Blu-ray, WEB, Vinyl, etc.) | All |
| `ArtistName` | Primary artist | Music folders/files |
| `AlbumTitle` | Album title | Music folders/files |
| `TrackTitle` | Track title | Music files |
| `TrackNumber` | Track number | Music files |
| `DiscNumber` | Disc number | Music files |
| `Extension` | File extension without leading dot | All |

Supported format suffixes:

- `:00`, `:000`, etc. – Zero-pad numeric values to the specified width.
- `:upper` – Convert to upper case.
- `:lower` – Convert to lower case.
- `:title` – Apply English title casing.

Whitespace is normalized, illegal filesystem characters are stripped, and empty
brackets/parentheses are pruned after substitution.

## Running Rename Jobs
Renames are initiated through `StartRenameJob` on the library service. The call
returns a `RenameJobRecord` describing the operations that ran, the templates in
use, and the unique job ID. When `DryRun` is `true`, the method produces the
same response without mutating state or recording history.

```go
record, err := svc.StartRenameJob(ctx, entityID, library.RenameOptions{})
```

For previews, use `PreviewRename` which produces a `RenamePreview` with the
same operation payload but no job metadata. Both APIs reject entities that lack
registered files and surface template validation issues early.

## History Awareness & Auditing
The renamer stores a per-file `renameState` entry capturing the last job ID,
target path, and completion timestamp. Subsequent jobs reuse that information
to:

- Skip renames that already match the target path while annotating the prior
  job ID in the `Reason` field.
- Highlight when a file diverged from its historical path so operators know the
  rename is “restoring” a previous decision.
- Detect conflicting target paths before jobs execute.

`RenameHistory` exposes the recorded jobs ordered from newest to oldest and is
used by the web dashboard to display rename activity. Dry-run executions do not
modify the history log or the tracked file paths.

## Worker Execution Model
`StartRenameJob` updates the in-memory file catalog immediately for the
purposes of the test harness. In production deployments the job record will be
handed to workers that perform the actual filesystem moves. Each operation
contains the source/target path plus the last successful job metadata so worker
logs can emit rich audit trails.

## Operational Guidance

1. **Preview First:** Run `PreviewRename` (or `StartRenameJob` with `DryRun:
   true`) after adjusting templates to verify the rename surface.
2. **Roll Out Templates Gradually:** Because templates are global, stage
   updates in lower environments or apply them during maintenance windows to
   avoid churn in the worker queue.
3. **Handle Conflicts:** When `Reason` mentions target conflicts, adjust the
   templates or metadata to produce unique paths before dispatching another job.
4. **Track History in the Dashboard:** Surface the `RenameHistory` output in the
   UI so operators can audit when and why files were moved.

## Future Extensions
Planned enhancements include:

- Per-library template overrides so multi-tenant installations can customize
  patterns without affecting neighboring catalogs.
- Rule-based fallbacks (e.g., different patterns for anime, specials, or live
  albums).
- Distributed worker integration that batches filesystem operations and emits
  structured audit events for external archival.
