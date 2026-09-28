# deploy — local Docker dependencies

Single Compose entry: **`compose.yaml`** (Compose Spec + profiles).

```bash
# from repo root
make compose-up            # core only: postgres / redis / minio
make compose-all           # core + casdoor + langfuse
make compose-casdoor       # + Casdoor IAM
make compose-langfuse      # + Langfuse v4 (needs langfuse/.env)
make compose-down          # stop & remove all services in this project
```

Equivalent raw commands:

```bash
COMPOSE='docker compose -f deploy/compose.yaml --project-directory deploy'
$COMPOSE up -d
$COMPOSE --profile casdoor up -d
env -u DATABASE_URL $COMPOSE --profile langfuse --env-file deploy/langfuse/.env up -d
```

Fragments live under `compose/` (`core.yaml`, `casdoor.yaml`, `langfuse.yaml`); do not point `docker compose -f` at those alone unless debugging.

Sandbox images (`sandbox/`, `sandbox-desktop/`) are Dockerfiles only — not part of this compose project.

## Profiles & ports

| Profile | Services | Host ports |
|---------|----------|------------|
| *(default / always)* | `postgres`, `redis`, `minio` | `5432`, `6379`, `9000`/`9001` |
| `casdoor` | `casdoor-postgres`, `casdoor` | `5434`, `8000` |
| `langfuse` | `langfuse-postgres`, `langfuse-redis`, `langfuse-minio`, `clickhouse`, `langfuse-web`, `langfuse-worker` | `5433`, `6380`, `9090`/`9091`, `8123`, `3100`, `3030` |

## Keep beside compose

| Path | Role |
|------|------|
| `casdoor/conf/` | Casdoor `app.conf` (DB host = `casdoor-postgres`) |
| `casdoor/README.md` | OIDC setup notes |
| `langfuse/.env` / `.env.example` | Langfuse secrets; `LANGFUSE_DATABASE_URL` must use host `langfuse-postgres` |

## Migration from old multi-compose layout

Previously three projects:

| Old | New |
|-----|-----|
| `deploy/docker-compose.yml` (project ≈ `deploy`) | default services in `compose.yaml` |
| `deploy/casdoor/docker-compose.yml` (project `casdoor`) | `--profile casdoor` |
| `deploy/langfuse/docker-compose.yml` (project `langfuse`) | `--profile langfuse` |

**Core volumes** (`deploy_openbot_pg`, `deploy_openbot_minio`) keep the same project name (`deploy`) and should reuse automatically.

**Casdoor / Langfuse** used separate project names, so old volumes (`casdoor_casdoor_pg`, `langfuse_langfuse_*`) are **not** attached to the new `deploy_*` volumes. For local DX, easiest is recreate:

```bash
# stop old stacks if still running
docker compose --project-directory deploy/casdoor -f deploy/casdoor/docker-compose.yml down 2>/dev/null || true
docker compose --project-directory deploy/langfuse -f deploy/langfuse/docker-compose.yml down 2>/dev/null || true

make compose-casdoor     # fresh casdoor_pg under project deploy
make compose-langfuse    # fresh langfuse_* under project deploy
# Re-do Casdoor app client setup / accept Langfuse headless init again if volumes were discarded.
```

To keep old data, copy or re-attach volumes manually (`docker volume` inspect / `external: true`) — not covered here.
