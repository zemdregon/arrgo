# Exporting Backups from Sonarr, Radarr, and Lidarr

This runbook captures the exact steps used to export configuration and database backups from the legacy *arr services before migrating to arrgo. Follow it verbatim so the resulting archives can be restored if the migration needs to be rolled back.

## Prerequisites
- Administrative access to each Sonarr, Radarr, and Lidarr instance.
- API keys for the services (found under `Settings > General > Security`).
- `curl` and `jq` installed on the workstation performing the export.
- A directory on durable storage (e.g., object storage bucket, NAS share) dedicated to migration backups.

## 1. Create On-Demand Backups via the UI
1. Sign in to the service.
2. Navigate to `System > Backup`.
3. Click **Backup Now**.
4. Wait for the backup to appear in the Completed list and download the `.zip` archive.
5. Copy the archive to the shared migration backup directory. Use a naming convention such as `sonarr-backup-<YYYYMMDDHHMM>.zip`.
6. Repeat for Radarr and Lidarr.

> **Why this matters:** The UI flow ensures the full application database, configuration, and custom scripts are captured exactly as the service would restore them.

## 2. Automate Additional Exports via the API
Use the helper script below to collect API-driven backups and generate checksums. The script issues a POST to the `/api/v3/system/backup` endpoint exposed by each service and saves the resulting archive locally.

```bash
#!/usr/bin/env bash
set -euo pipefail

: "${BACKUP_DIR:?Set BACKUP_DIR to the destination directory}" 
SERVICES=("sonarr" "radarr" "lidarr")

for SERVICE in "${SERVICES[@]}"; do
  HOST_VAR="${SERVICE^^}_URL"
  KEY_VAR="${SERVICE^^}_API_KEY"
  HOST=${!HOST_VAR:?"Set $HOST_VAR to the base URL (e.g. https://sonarr.local:8989)"}
  API_KEY=${!KEY_VAR:?"Set $KEY_VAR to the API key"}

  TIMESTAMP=$(date -u +"%Y%m%dT%H%M%SZ")
  OUTPUT="${BACKUP_DIR}/${SERVICE}-backup-${TIMESTAMP}.zip"

  echo "Creating ${SERVICE} backup at ${OUTPUT}"
  curl -fsSL -X POST "${HOST}/api/v3/system/backup" \
    -H "X-Api-Key: ${API_KEY}" \
    --output "${OUTPUT}"

  sha256sum "${OUTPUT}" | tee "${OUTPUT}.sha256"

done
```

1. Save the script as `export_servarr_backups.sh` on an administration host.
2. `chmod +x export_servarr_backups.sh`
3. Export the required environment variables:
   ```bash
   export BACKUP_DIR="/srv/arrgo-migration/backups"
   export SONARR_URL="https://sonarr.local:8989"
   export SONARR_API_KEY="<api-key>"
   export RADARR_URL="https://radarr.local:7878"
   export RADARR_API_KEY="<api-key>"
   export LIDARR_URL="https://lidarr.local:8686"
   export LIDARR_API_KEY="<api-key>"
   ```
4. Run the script and verify `.zip` and `.sha256` files are produced for each service.
5. Copy the artifacts to your durable backup location.

## 3. Verify Backup Integrity
- Run `sha256sum -c *.sha256` in the backup directory to validate the files have not changed.
- Test extracting one archive from each service to confirm the contents (e.g., `tar -tf sonarr-backup-20250301T153000Z.zip`).
- Record the backup timestamps, file sizes, and checksum output in the migration tracking sheet.

## 4. Store Metadata for Future Restores
Create (or update) a `BACKUP_MANIFEST.md` file alongside the archives with:
- Service name and version.
- Backup timestamp.
- Location of the stored archive.
- Checksum output.
- Operator who performed the export.

This documentation provides accountability and accelerates restore operations if a rollback is required.

## Completion Criteria
The migration checklist item “Backups exported for each *arr service” is satisfied when:
- UI-triggered backups have been downloaded for Sonarr, Radarr, and Lidarr.
- API-driven backups are archived with corresponding SHA-256 checksums.
- A manifest captures metadata about each archive.
- Artifacts are stored on resilient storage accessible to the migration team.
