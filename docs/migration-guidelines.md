# Migration Guidelines from Sonarr/Radarr/Lidarr to arrgo

These guidelines help existing Sonarr, Radarr, and Lidarr users move their media automation workflows onto arrgo. They assume you manage one or more *arr deployments today and are ready to consolidate catalog automation, download orchestration, and monitoring into a single arrgo cluster.

## Audience and Prerequisites
- Familiarity with operating the existing *arr applications, their automation policies, and connected services such as download clients, indexers, and notifications.
- Administrative access to the underlying infrastructure (Docker hosts, Kubernetes cluster, NAS, etc.) where the current stack resides.
- arrgo master and worker images published to a registry accessible from your environment. Follow the [development setup guide](development-setup.md) if you are standing up arrgo for the first time.

## High-Level Migration Flow
1. **Inventory the existing environment.** Capture versions, enabled automation features, and third-party integrations across Sonarr, Radarr, and Lidarr.
2. **Back up your data.** Export application configuration, quality profiles, and library metadata from each *arr instance. Retain copies of database backups and API keys until the migration is fully validated.
3. **Provision the arrgo cluster.** Deploy the PostgreSQL, Redis, and optional OpenSearch services, then start the arrgo master and required worker pools.
4. **Translate configuration.** Map your existing profiles, root folders, tags, and release restrictions into arrgo equivalents.
5. **Import catalogs.** Seed arrgo with your media inventory and historical activity.
6. **Reconnect automation.** Point download clients, indexers, and notification channels at arrgo.
7. **Validate and cut over.** Run verification scans, confirm automation works end-to-end, then decommission the legacy services.

## Detailed Steps

### 1. Inventory and Backups
- Document all media root paths, naming conventions, quality profiles, indexers, notification endpoints, and download clients that each *arr application uses.
- Generate configuration backups from Sonarr, Radarr, and Lidarr (`System > Backup`). Store the archives in versioned storage alongside exported API keys and webhook secrets.
- Note any custom scripts or post-processing hooks so they can be reimplemented through arrgo workers or webhook automation.

### 2. Prepare arrgo Infrastructure
- Stand up the required data services (PostgreSQL, Redis, optional OpenSearch) using the [`deploy/compose`](../deploy/compose) assets or equivalent infrastructure-as-code.
- Launch an arrgo master container configured with connection strings, API secrets, and cluster metadata.
- Start worker pools that align with your hardware capabilities (e.g., GPU-enabled transcode workers, dedicated music metadata workers).
- Configure observability endpoints (Prometheus, Grafana, log shipping) so you can monitor the migration in real time.

### 3. Map Domain Concepts
- **Media entities:**
  - Sonarr series → arrgo series entities.
  - Radarr movies → arrgo film entities.
  - Lidarr artists/albums/tracks → arrgo artist, album, and track entities.
- **Quality and profiles:** Translate Sonarr/Radarr/Lidarr quality profiles into arrgo policy profiles. Preserve cutoff thresholds, preferred codecs, and language filters.
- **Root folders and tags:** Align media root paths and tag-based automation rules with arrgo’s library configuration so imports land in the correct locations.
- **Automation rules:** Migrate release restrictions, delay profiles, and connect webhook/notification rules to arrgo equivalents.

### 4. Seed arrgo with Library Metadata
- Use arrgo’s import APIs or CLI tooling (when available) to ingest the exported metadata from Sonarr, Radarr, and Lidarr. Start with series catalogs, then movies, followed by music libraries.
- For each catalog, run an arrgo library scan to reconcile the physical files on disk with imported metadata. Resolve any mismatches before proceeding.
- Import historical activity (grabbed, downloaded, upgraded events) if you rely on automation history or reporting. arrgo accepts batch history uploads via its ingestion endpoints.

### 5. Reconnect Integrations
- Reconfigure download clients (e.g., qBittorrent, NZBGet) to use arrgo-generated API keys and webhook callbacks.
- Update indexer credentials and category mappings to point at arrgo’s integration layer.
- Mirror notification settings (Discord, Slack, email, webhook targets) so end users continue receiving status updates from arrgo.
- For downstream automation scripts, switch to arrgo’s REST/gRPC endpoints and adjust payload schemas as documented in [`docs/api-first-iteration.md`](api-first-iteration.md).

### 6. Verification and Cutover
- Schedule a dry run where both the legacy *arr stack and arrgo monitor the same libraries. Ensure arrgo does not trigger duplicate downloads during this shadow period.
- Validate that newly grabbed releases flow through download clients, import back into the correct media folders, and update metadata within arrgo.
- Compare arrgo quality upgrades and retention policies against expectations. Tune policy definitions until automation matches previous behavior or desired improvements.
- Once satisfied, disable automation in the legacy applications, keep them read-only for a safety window, then decommission when confident.

### 7. Post-Migration Tasks
- Archive legacy backups securely for at least one release cycle in case rollback is needed.
- Capture updated architecture diagrams and runbooks that reflect the arrgo topology.
- Enable ongoing health checks, metrics, and tracing so operations teams have visibility into the consolidated platform.
- Gather feedback from stakeholders, log follow-up tasks (e.g., advanced analytics, UI customizations), and prioritize them in the project roadmap.

## Rollback and Contingency Planning
- Maintain the legacy services in a powered-off but recoverable state until arrgo has run successfully through multiple automation cycles.
- Keep DNS/API endpoints abstracted (e.g., behind reverse proxies or service meshes) so you can shift traffic back if necessary.
- If critical issues arise, restore the latest Sonarr/Radarr/Lidarr backups, re-enable automation, and treat arrgo as a staging environment until remediation is complete.

## Support Channels
- Open arrgo GitHub issues for bugs or migration blockers discovered during the process.
- Share success stories or lessons learned in community forums to help shape future tooling, importers, and automation enhancements.

## Appendix: Quick Reference Checklist
- [ ] Backups exported for each *arr service
- [ ] arrgo data services deployed and reachable
- [ ] Master and worker pools configured with secrets and observability
- [ ] Media libraries mapped to arrgo entities and policies
- [ ] Metadata imported and reconciled with on-disk files
- [ ] Integrations (download clients, indexers, notifications) re-pointed
- [ ] Dry-run validation complete
- [ ] Legacy services decommissioned after observation window
