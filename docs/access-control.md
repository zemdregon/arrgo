# Access Control and API Keys

arrgo secures its HTTP surfaces with role-based access control (RBAC) backed by
labeled API keys. Every integration, automation, or user interface receives a
dedicated credential so operators can audit activity, rotate secrets
selectively, and scope permissions to the minimum required actions.

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
