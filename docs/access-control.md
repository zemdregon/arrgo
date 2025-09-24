# Access Control and API Keys

> **Last reviewed:** 2025-03-08  
> **Status:** Implemented for alpha builds (RBAC and API key workflows are wired into the HTTP server and dashboard)

arrgo secures its HTTP surfaces with role-based access control (RBAC) backed by
labeled API keys. Every integration, automation, or user interface receives a
dedicated credential so operators can audit activity, rotate secrets
selectively, and scope permissions to the minimum required actions.

## Implementation Snapshot
- RBAC enforcement lives in the `internal/auth` package and is invoked by both
  REST and gRPC middleware before handlers execute.
- API key issuance flows are exposed in the dashboard and through the
  `auth.Service` so that automation scripts can self-manage credentials.
- Audit metadata (creator, last rotation timestamp, revocation reason) is stored
  alongside hashed secrets for downstream analytics.

## Roles and Permissions

The platform ships with the following built-in roles:

| Role | Description | Permissions |
| --- | --- | --- |
| `admin` | Full control over library orchestration, worker pools, and identity management. | `library:read`, `library:write`, `worker:read`, `worker:manage`, `identity:admin` |
| `operator` | Manage library jobs and worker pools without changing identity settings. | `library:read`, `library:write`, `worker:read`, `worker:manage` |
| `observer` | Read-only access to catalog metadata and worker status. | `library:read`, `worker:read` |

Additional roles can be registered programmatically using the `auth.Service`
apis if bespoke permission groupings are required.

## Managing API Keys

API keys pair a human-friendly label with a role assignment. The label should be
unique per integration (e.g., `dashboard`, `plex-automation`) so that operators
know which credential to rotate or revoke. Key management workflows:

- **Create:** `Service.CreateAPIKey` returns a one-time plaintext token in the
  form `<key-id>.<secret>`. Store this value securely; arrgo only persists a
  salted hash.
- **Rotate:** `Service.RotateAPIKey` issues a new secret for an existing key and
  optionally updates its label. The previous token becomes invalid immediately.
- **Revoke:** `Service.RevokeAPIKey` disables a key so that future requests are
  rejected.
- **List:** `Service.ListAPIKeys` surfaces metadata (label, role, status,
  timestamps) for auditing.

### Presenting Tokens

HTTP clients must send the full token using the `Authorization: Bearer` header
or the `X-Arrgo-Api-Key` header. Tokens are composed of the key identifier and
secret separated by a dot. During authentication the secret is hashed and the
role's permissions are evaluated before routing the request.

### Permission Mapping

REST endpoints enforce the following permissions:

| Endpoint | Permission |
| --- | --- |
| `GET /api/v1/library/entities` | `library:read` |
| `GET /api/v1/jobs` | `library:read` |
| `POST /api/v1/jobs` | `library:write` |
| `GET /api/v1/workers` | `worker:read` |
| `POST /api/v1/workers/register` | `worker:manage` |
| `POST /api/v1/workers/{id}/heartbeat` | `worker:manage` |

Observability surfaces (`/metrics`, `/debug/pprof/*`) remain unauthenticated so
operators can integrate them with existing telemetry pipelines.

## Operational Guidance

- Issue separate keys per external system to minimize blast radius and simplify
  auditing.
- Schedule key rotations periodically (e.g., every 90 days) and after any
  suspected credential exposure.
- Revoke unused keys promptly; authentication attempts with revoked tokens
  return `401` responses so monitoring can alert on stale integrations.
- Extend the RBAC model through new roles if a workflow requires a bespoke
  blend of read/write capabilities.

## Audit & Compliance Considerations
- **Logging:** Authentication middleware emits structured logs for every
  successful and failed request, including key ID, role, and HTTP status code.
- **Metrics:** Prometheus counters track per-role request volume, rejected
  requests, and rotation events. Alert on spikes in `401` responses to detect
  leaked keys.
- **Dashboards:** The dashboard surfaces key age, last used timestamp, and the
  services calling each credential so that compliance checks can spot dormant
  integrations.

## Future Enhancements
- Integrate single sign-on providers and map identity provider groups to arrgo
  roles for organizations that prefer centralized identity management.
- Allow scoped API keys with fine-grained permissions (e.g., read-only access
  to specific libraries) to support delegated automation.
- Provide built-in rotation reminders and exportable audit trails for regulated
  environments.
