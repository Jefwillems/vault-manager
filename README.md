# vault-manager

A nightly job that uses the [GitHub Copilot SDK](https://github.com/github/copilot-sdk) to automatically organize free-form "braindump" notes into a structured [Obsidian](https://obsidian.md) knowledge base.

## How it works

Throughout the day you drop raw, unstructured notes (braindumps) into a designated inbox folder inside your vault. At night, vault-manager:

1. **Scans** the braindump inbox for notes with `processed: false` in their frontmatter.
2. **Launches** an embedded Copilot agent (the *Vault Librarian*) with file-only access to the vault.
3. **Processes** each braindump — extracting people, meetings, decisions (ADRs), and action items, then folding them into the vault's existing structure using `[[wikilinks]]` for cross-referencing.
4. **Writes** a dated run summary to the vault's `History/` folder.
5. **Archives** processed braindumps to `Archive/Braindumps/` once the agent finishes.

The vault's folder structure is self-describing: each top-level folder carries a `README.md` that defines its purpose, filename conventions, and frontmatter schema. The agent reads these at runtime rather than relying on any hardcoded layout. The overall contract between the harness and the agent is defined in an `AGENTS.md` file at the vault root.

### Security model

The agent runs with a restrictive permission policy — file reads and writes are allowed, but shell commands and network requests are denied. This prevents braindump content from coaxing the agent into exfiltrating credentials or running arbitrary commands.

## Prerequisites

- A GitHub account with an active **GitHub Copilot** subscription.
- A personal access token (or similar) with Copilot API access.
- A vault directory structured with an `AGENTS.md` and per-folder `README.md` files.

## Configuration

All configuration is via environment variables:

| Variable                  | Default                | Description                                                                 |
|---------------------------|------------------------|-----------------------------------------------------------------------------|
| `COPILOT_GITHUB_TOKEN`    | *(required)*           | GitHub token used to authenticate with the Copilot API.                     |
| `VAULT_PATH`              | `/app/data/vault`      | Absolute path to the vault directory.                                       |
| `BRAINDUMP_DIR`           | `Braindumps`           | Vault-relative folder that acts as the braindump inbox.                     |
| `COPILOT_MODEL`           | `claude-sonnet-4.5`    | Copilot model to use for the agent session.                                 |
| `COPILOT_REASONING_EFFORT`| *(empty)*              | Reasoning effort for supported models (`low`\|`medium`\|`high`\|`xhigh`).  |
| `RUN_TIMEOUT`             | `30m`                  | Maximum wall-clock time for a single run (Go duration string).              |
| `LOG_LEVEL`               | `info`                 | Log verbosity (`debug`\|`info`\|`warn`\|`error`).                          |
| `FORCE`                   | `false`                | Run the agent even when no unprocessed braindumps are found.                |
| `PUSHGATEWAY_URL`         | *(empty)*              | Base URL of a Prometheus Pushgateway. Empty disables metrics.               |
| `PUSHGATEWAY_JOB`         | `vault-manager`        | Pushgateway `job` grouping label for pushed metrics.                        |
| `INSTANCE_ID`             | *(hostname)*           | Pushgateway `instance` grouping label (defaults to the pod/host name).      |

## Running with Docker

```bash
docker run --rm \
  -e COPILOT_GITHUB_TOKEN=ghp_... \
  -e VAULT_PATH=/vault \
  -v /path/to/your/vault:/vault \
  ghcr.io/jefwillems/vault-manager:latest
```

## Kubernetes CronJob

A typical deployment mounts the vault from a PVC (e.g. synced by [obsidian-livesync](https://github.com/vrtmrz/obsidian-livesync)'s CouchDB bridge) and runs vault-manager on a nightly schedule:

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: vault-manager
spec:
  schedule: "0 2 * * *"   # 02:00 every night
  jobTemplate:
    spec:
      template:
        spec:
          restartPolicy: OnFailure
          containers:
            - name: vault-manager
              image: ghcr.io/jefwillems/vault-manager:latest
              env:
                - name: COPILOT_GITHUB_TOKEN
                  valueFrom:
                    secretKeyRef:
                      name: copilot-token
                      key: token
                - name: VAULT_PATH
                  value: /app/data/vault
              volumeMounts:
                - name: vault
                  mountPath: /app/data/vault
          volumes:
            - name: vault
              persistentVolumeClaim:
                claimName: obsidian-vault
```

Pin the image to a specific digest (printed as a workflow summary on each push to `main`) for reproducible deployments.

## Metrics & observability

vault-manager is a short-lived CronJob: it starts, processes the vault, and exits. Prometheus *scrapes* long-running targets, so it can never scrape a job that has already terminated. The standard pattern for batch/cron workloads is the **[Prometheus Pushgateway](https://github.com/prometheus/pushgateway)**: at the end of each run vault-manager *pushes* its outcome to the gateway, and Prometheus scrapes the gateway on its own schedule. Each run overwrites the previous sample for its grouping key (`job`/`instance`), so the gateway always reflects the most recent run.

Metrics are opt-in: set `PUSHGATEWAY_URL` to enable them; leave it empty and the harness runs unchanged with no monitoring dependency.

Pushed gauges (job=`vault-manager`, instance=pod name):

| Metric                                      | Meaning                                                        |
|---------------------------------------------|----------------------------------------------------------------|
| `vault_manager_last_run_success`            | `1` if the last run succeeded, `0` if it failed.               |
| `vault_manager_last_run_timestamp_seconds`  | Unix time the last run completed (alert on staleness).         |
| `vault_manager_run_duration_seconds`        | Wall-clock duration of the last run.                           |
| `vault_manager_braindumps_unprocessed`      | Unprocessed braindumps found at the start of the run.          |
| `vault_manager_braindumps_archived`         | Braindumps archived (processed) during the run.                |

Example alerts on a failed or overdue run:

```yaml
- alert: VaultManagerRunFailed
  expr: vault_manager_last_run_success == 0
  for: 5m
- alert: VaultManagerRunStale
  expr: time() - vault_manager_last_run_timestamp_seconds > 172800  # >48h since last run
```

### Deploying

Ready-to-apply manifests live in [`deploy/`](deploy/):

- [`deploy/cronjob.yaml`](deploy/cronjob.yaml) — the CronJob with the `PUSHGATEWAY_*` env vars wired in.
- [`deploy/grafana-dashboard-configmap.yaml`](deploy/grafana-dashboard-configmap.yaml) — a Grafana dashboard delivered as a ConfigMap (labelled `grafana_dashboard: "1"` so the Grafana sidecar auto-imports it). The raw dashboard is [`deploy/grafana-dashboard.json`](deploy/grafana-dashboard.json).

```bash
kubectl apply -f deploy/cronjob.yaml
kubectl apply -f deploy/grafana-dashboard-configmap.yaml   # into your Grafana sidecar's namespace
```

You need a Pushgateway reachable at `PUSHGATEWAY_URL` (e.g. the `prometheus-pushgateway` chart, or the one bundled with kube-prometheus-stack) and Prometheus configured to scrape it.

## Building from source

The binary embeds the Copilot CLI via the SDK bundler — no separate `copilot` binary is needed at runtime.

```bash
# Embed the CLI for the current platform and build
go tool bundler --platform linux/amd64 --output cmd/vault-manager
CGO_ENABLED=0 go build -o vault-manager ./cmd/vault-manager
```

Or just build the Docker image, which handles all of this automatically:

```bash
docker build -t vault-manager .
```

## CI

Pushes to `main` and manual triggers build a multi-tag Docker image and push it to GHCR (`ghcr.io/jefwillems/vault-manager`). The workflow uses Docker Buildx and attaches the image digest to the workflow summary.

## Vault conventions

vault-manager expects the vault to follow a self-describing layout:

- **`AGENTS.md`** at the vault root — defines the schema, frontmatter conventions, folder routing rules, and the provenance/wikilink contract.
- **`<Folder>/README.md`** in each content folder — defines that folder's purpose, filename pattern, and frontmatter schema.
- **Braindump frontmatter** — each braindump note must include `processed: false` to be picked up. After processing, the agent sets it to `processed: true` and the harness moves the file to `Archive/Braindumps/`.

## License

See [LICENSE](LICENSE).
