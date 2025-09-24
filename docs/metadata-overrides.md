# Metadata Override Toolkit

arrgo exposes first-class tools that let operators reshape catalog metadata without waiting on upstream providers. Overrides
are persisted alongside library entities so workflow automation, renaming pipelines, and dashboard views stay consistent with
manual corrections.

## Supported Override Types

### Episode and Season Cutovers
- Realign split seasons or alternate broadcast orders by specifying the desired season/episode numbers.
- Adjust absolute numbering for anime or daily programming.
- Capture regional titles and special air dates to keep dashboards and notifications accurate.

### Album Editions
- Mark deluxe, anniversary, or remastered editions as the preferred default for automation.
- Document bonus content and packaging notes so downstream jobs (e.g., renaming) reference the correct variant.

### Track Numbering
- Override disc assignments and track numbers when release groups disagree with canonical databases.
- Ensure consistent ordering across library imports, even when upstream metadata is inconsistent.

## API Endpoints

The REST API surfaces CRUD operations under `/api/v1/library/overrides/{entity_id}`:

| Method | Description |
| --- | --- |
| `GET` | Fetch the current overrides for an entity. Returns `{"overrides": null}` when no overrides are present. |
| `PUT` | Replace the override set. Episode, album, and track payloads may be provided independently. |
| `DELETE` | Remove overrides so the entity falls back to provider-supplied metadata. |

Example `PUT` request:

```bash
curl -X PUT \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  http://localhost:8080/api/v1/library/overrides/series-1 \
  -d '{
        "episode_cutovers": [
          {"episode_id": "s01e01", "season_number": 1, "episode_number": 1, "title": "Pilot"}
        ],
        "album_editions": [
          {"edition_id": "deluxe", "name": "Deluxe", "is_default": true}
        ],
        "track_number_overrides": [
          {"track_id": "track-1", "disc_number": 1, "track_number": 1}
        ]
      }'
```

## Service Behavior

- Overrides are validated, normalized, and timestamped before storage so responses always return sorted, immutable data.
- `UpdatedAt` reflects the service clock, making it easy to audit manual adjustments in downstream tooling.
- Clearing overrides immediately restores provider metadata without requiring a refresh job.

Refer to the implementation details in [`internal/library/metadata_overrides.go`](../internal/library/metadata_overrides.go)
and the REST wiring in [`internal/api/httpapi/server.go`](../internal/api/httpapi/server.go) for full struct definitions and
validation rules.
