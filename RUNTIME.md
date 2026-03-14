# Accelerated Container Runtime

RWX is powered by a purpose-built container runtime designed for speed and parallelism. It rethinks CI and container image builds from first principles — replacing the VM-per-job model and sequential Dockerfile layers with graph-based task execution across distributed infrastructure.

## How it works

Traditional CI systems run steps sequentially inside a single VM or container. RWX takes a different approach: tasks execute on separate machines simultaneously, each with right-sized compute, and results are combined through content-based caching.

### Graph-based scheduling

Tasks are defined as a directed acyclic graph (DAG). The runtime analyzes dependencies and schedules independent tasks across distributed infrastructure in parallel. Only tasks whose inputs have changed are re-executed — unchanged tasks resolve instantly from cache.

This is a fundamental departure from systems like GitHub Actions, where parallelization requires manually partitioning work across separate jobs — each with duplicated setup steps. In RWX, maximum parallelization comes from the dependency graph itself. Engineers define what depends on what; the runtime handles the rest.

Pipeline definitions can also be generated at runtime, enabling dynamic workflows driven by code rather than static YAML.

### Content-based caching with sandboxing

Rather than caching by key name or step order, the runtime caches based on actual filesystem contents. Tasks are sandboxed so that only files matching a `filter` specification exist on disk during execution. This prevents unrelated file changes from busting the cache and eliminates the manual `hashFiles` key management required by traditional CI.

This design means:

- **Cache hits survive misses** — individual tasks cache independently, so a miss on one task doesn't invalidate downstream tasks
- **No all-or-nothing invalidation** — unlike BuildKit, where a single changed layer rebuilds everything below it, RWX can resume cache hits after a miss
- **Cross-run reuse** — cache is shared across branches, PRs, and local CLI runs
- **The entire registry is the cache** — rather than limited per-branch or per-runner caches, the full history of cached results is available

### Per-task compute

Each task specifies its own resource requirements. The runtime allocates the exact CPU and memory needed per task rather than provisioning a fixed-size runner for the entire pipeline. A compilation step can claim 16 cores while a linting step runs on 2 — simultaneously, on separate machines. Tasks scale up to 64 cores.

### Minimal base images

RWX uses minimal container images rather than the bloated VM images common in traditional CI (GitHub Actions ships a 74 GiB image with hundreds of pre-installed packages). Dependencies are explicit, declared per-task, making builds portable and reproducible.

## Building container images without Dockerfiles

RWX can build OCI-compatible container images without Dockerfiles or BuildKit. Instead of sequential `RUN` / `COPY` layers, images are defined as a graph of tasks — the same primitive used for CI pipelines.

### Why replace Dockerfiles

Dockerfiles have fundamental limitations:

- **Build context overhead** — most builds upload the entire repository into the build context, wasting time especially with remote builders
- **Sequential execution** — layers execute one after another, even when they have no dependency on each other
- **Fragile caching** — cache invalidation cascades: one changed layer rebuilds everything below it, forcing developers into tedious `COPY` ordering tricks
- **Multi-stage complexity** — multi-stage builds help with parallelism but require manual `COPY --from` file transfers between stages

### How task-based image builds work

The runtime saves the filesystem changes from every task, producing a container layer at each step. These layers compose into a final OCI-compatible image that can be pulled with `docker pull` or `rwx image pull`.

A Dockerfile like this:

```dockerfile
FROM node:24-slim AS builder
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM node:24-slim AS runtime
COPY --from=builder /app/dist ./dist
CMD ["node", "dist/index.js"]
```

Becomes a set of tasks:

```yaml
base:
  image: node:24-slim
  config: none

tasks:
  - key: code
    run: git clone ...

  - key: npm-install
    use: code
    run: npm ci
    filter:
      - package.json
      - package-lock.json

  - key: build
    use: code
    run: npm run build
    outputs:
      artifacts:
        - key: dist
          path: dist

  - key: dist
    run: cp -a ${{ tasks.build.artifacts.dist }}/. ./dist

  - key: image
    use: dist
    run: echo "node dist/index.js" | tee $RWX_IMAGE/command
```

Key differences from Dockerfiles:

- `npm-install` and `build` can run in parallel on separate machines
- `filter` ensures only `package.json` and `package-lock.json` affect the cache for `npm-install` — changes to source files don't trigger a reinstall
- Artifacts replace `COPY --from` for transferring files between stages
- Image metadata (`CMD`, `ENTRYPOINT`, `ENV`, `WORKDIR`) is set by writing to files under `$RWX_IMAGE/`

### No compression overhead

Cloud networks are substantially faster than compression algorithms. The runtime stores and transmits layers uncompressed — it is faster to upload 1 GB of data than to gzip 1 GB of data. Storage cost is treated as negligible compared to compute cost.

### No build context uploads

Instead of uploading the repository into a build context, RWX clones the repository directly on the build machine using its [`git/clone` package](https://www.rwx.com/docs/rwx/packages/git/clone/2.0.0). This avoids transferring the entire repo over the network before the build even starts.

## Dockerfile instruction mapping

For teams migrating from Dockerfiles, each instruction has a direct equivalent:

| Dockerfile | RWX |
|---|---|
| `FROM` | `base.image` in YAML config |
| `RUN` | Multi-line `run` scripts in tasks |
| `COPY` / `ADD` | `git/clone` package + `filter` |
| `CMD` | Write to `$RWX_IMAGE/command` or `command.json` |
| `ENTRYPOINT` | Write to `$RWX_IMAGE/entrypoint` or `entrypoint.json` |
| `ENV` | Task-level `env` configuration |
| `ARG` | Init parameters with `${{ init.param-name }}` |
| `WORKDIR` | Write path to `$RWX_IMAGE/workspace` |
| `LABEL` | Write to `$RWX_IMAGE/labels/{key}` |
| `USER` | Write username to `$RWX_IMAGE/user` |
| `SHELL` | Write to `$RWX_IMAGE/shell` |

See the full [Migrating from Dockerfile](https://www.rwx.com/docs/migrating-from-dockerfile) guide for detailed examples.

## Learn more

- [CI/CD Platform](https://www.rwx.com/ci-cd)
- [Container Image Builds](https://www.rwx.com/container-image-builds)
- [Migrating from Dockerfile](https://www.rwx.com/docs/migrating-from-dockerfile)
- [What would GitHub Actions look like if you designed it today?](https://www.rwx.com/blog/what-would-github-actions-look-like-if-you-designed-it-today)
- [We deleted our Dockerfiles](https://www.rwx.com/blog/we-deleted-our-dockerfiles)
- [Proposal for a new way to build container images](https://www.rwx.com/blog/proposal-for-a-new-way-to-build-container-images)
- [Documentation](https://www.rwx.com/docs)
