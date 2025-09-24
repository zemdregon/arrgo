# arrgo Development Setup

> **Last reviewed:** 2025-03-08  
> **Status:** Up to date for alpha development environments

This guide helps contributors quickly bootstrap a local arrgo environment for experimentation and feature work.

## Prerequisites
- **Go 1.23+** installed and added to your `PATH` (the module is pinned to Go 1.23).
- **Node.js 18+ and npm** for building and running the web dashboard (`web/dashboard`).
- **Docker** and **Docker Compose v2** for running the distributed stack.
- **Make** (optional) for running common automation commands when they become available.
- Access to a Unix-like shell (macOS, Linux, or WSL2 on Windows).

## Repository Bootstrapping
1. Clone the repository: `git clone https://github.com/<your-org>/arrgo.git`.
2. Change into the project directory: `cd arrgo`.
3. Ensure submodules are initialized if they are introduced in the future: `git submodule update --init --recursive`.

## Configure Environment Variables
Copy the example environment file to create local overrides:

```bash
cd deploy/compose
cp .env.example .env
```

Adjust the values in `.env` for image tags, port mappings, and service credentials as needed. The defaults work for most local workflows.

## Launch the Docker Compose Stack
From the `deploy/compose` directory:

```bash
docker compose up --build
```

The command provisions PostgreSQL, Redis, OpenSearch, arrgo master/worker
containers, and the dashboard UI with sensible defaults. Access the dashboard at
<http://localhost:3000>. To simulate a larger cluster, scale workers:

```bash
docker compose up --build --scale arrgo-worker=3
```

The stack uses named volumes so data survives container restarts. Use `docker compose down -v` to stop the stack and remove volumes when you need a clean slate.

> **API token:** The Compose file sets `ARRGO_BOOTSTRAP_ADMIN_TOKEN` (default
> `arrgo-admin.devtoken`) so the dashboard and workers can authenticate against
> the master API. Override the environment variable in `deploy/compose/.env` to
> supply a custom credential.

## Go Tooling
- Run `go fmt ./...` to format Go sources.
- Execute `go test ./...` before pushing changes.
- Consider installing `golangci-lint` for advanced linting once configured for the repository.

### Recommended Extras
- Install [`air`](https://github.com/cosmtrek/air) or a similar live-reload tool for rapid service iteration.
- Use [`direnv`](https://direnv.net/) to manage environment variables when switching between deployments.
- Configure [`buf`](https://buf.build/) locally if you plan to work on protobufs; the repository includes `buf.yaml` scaffolding.

## Web Dashboard Development
The read-only cluster dashboard lives in [`web/dashboard`](../web/dashboard) and is implemented with Next.js and React.

1. Install dependencies: `cd web/dashboard && npm install`.
2. Start the development server: `npm run dev` (defaults to <http://localhost:3000>).
3. Point the dashboard at your API host by setting `NEXT_PUBLIC_ARRGO_API_URL` (defaults to `http://localhost:8080`).
4. Run linting with `npm run lint` and build with `npm run build`. Use `npm start` to serve the production build locally.

The dashboard fetches data from the REST API exposed by the Go services (`/api/v1/library/entities`, `/api/v1/jobs`, `/api/v1/workers`). Each panel uses reusable components so additional views can piggyback on the existing layout and status badges.

> **Tip:** The Docker Compose dashboard service automatically sets
> `NEXT_PUBLIC_ARRGO_API_URL` to the internal master service URL so the UI works
> out of the box.

## Observability During Development
- Access container logs with `docker compose logs -f <service>`.
- When Prometheus/Grafana assets are introduced, expose their ports locally for dashboard access.
- Use structured logging fields to correlate master and worker events.

## Troubleshooting Tips
- If ports collide with existing services, adjust them in `deploy/compose/*.yml` or override via `.env`.
- Ensure Docker has sufficient CPU and memory assigned (at least 4 CPUs / 6 GB RAM recommended).
- Remove stale volumes if migrations or schema changes fail: `docker volume prune`.

## Contribution Checklist
Before opening a pull request:

1. Run `go test ./...` and any relevant front-end checks (`npm run lint`, `npm run test`).
2. Update documentation in `docs/` when behavior changes or new features are introduced.
3. Verify Docker Compose launches successfully after your modifications.
4. Capture architectural decisions as ADRs if they introduce new dependencies or protocols.

## Next Steps
With the environment running, begin iterating on arrgo services, write tests, and document findings in Architecture Decision Records (ADRs) when design decisions change the platform trajectory.
