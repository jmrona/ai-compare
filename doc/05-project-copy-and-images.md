# 5. Project copy and side images

Code: `backend/internal/workspace/workspace.go` (copy, inspection, folder browser, side images), `result.go` (result images, collection, tests, live diff, clean-up), `collect-result.sh`, and the helper image in `backend/internal/workspace/copier/`.

## The problem

`api` runs in a Linux container. The user's project is on the host (`C:\Users\…` on Windows, `/Users/…` on macOS, `/home/…` on Linux). `api` cannot see that folder, and mounting every possible project folder into `api` in advance is impossible.

The solution relies on **Docker-out-of-Docker**: `api` talks to the host's Docker daemon through the mounted socket, and bind-mount paths are resolved **by the daemon, on the host**. So `api` can create a container that mounts `C:\` even though `api` itself has no `C:\`.

## The copy helper

A tiny Alpine image (`git`, `tar`, `grep`, `findutils`, `coreutils`, `sed`) with three scripts:

- `copy-project.sh` — copies a host folder into the staging volume: the project, or the hidden tests. It takes the comparison id, the destination name (`project` or `hidden`) and the path;
- `inspect-project.sh` — reports what would be copied, without copying;
- `list-folders.sh` — lists the sub-folders of a folder, for the folder browser.

The files are embedded in the Go binary (`//go:embed`). The image is built on first use and tagged with a hash of its files (`ai-compare/copier:<12 hex>`), so changing a script automatically produces a new image.

### Mounting: the top-level folder, read-only

`splitHostPath` splits the user's path into an **anchor** and the rest:

| Path | Anchor (mounted at `/host`) | Rest (passed as argument) |
|---|---|---|
| `C:\Users\me\app` | `C:\` | `Users/me/app` |
| `/Users/me/app` | `/Users` | `me/app` |
| `/home/me/app` | `/home` | `me/app` |

The anchor is bind-mounted **read-only** at `/host`, and the script works on `/host/<rest>`.

Why not mount the project folder directly? **Docker Desktop on Windows creates a bind source that does not exist.** A typo in the path would silently create an empty folder on the user's disk and copy nothing. Mounting the anchor (which always exists) and checking the path inside the container gives a clear error instead and never creates anything.

Errors are mapped to clear messages:

| Situation | Signal | Message |
|---|---|---|
| Path does not exist or is not a folder | script exits 3 | "the project folder does not exist" |
| Path cannot be read | script exits 4 | "docker cannot access this folder" |
| Docker refuses the mount (folder not shared with Docker Desktop, e.g. `/Volumes` on macOS) | daemon error at create/start | Same, with "Docker Desktop: Settings → Resources → File sharing" |
| Relative path or `/` | Go validation | "the project path must be absolute" |

### What gets copied

- **Git repositories:** `git ls-files --cached --others --exclude-standard`, i.e. tracked files plus untracked files that `.gitignore` does not exclude. That is the folder **as it is, including uncommitted changes**. `.git` itself is not copied (each side gets a fresh baseline instead). `safe.directory='*'` avoids Git's ownership check on the mounted folder.
- **Other folders:** everything except `.git`, `node_modules`, `dist`, `build`, `.venv`, `venv`, `target` and `__pycache__`.
- **Never copied:** `.env` files and their variants (`.env.local`, `config/.env.production`…), because their contents would be read by the agent and sent to the provider. **Templates are kept:** `.env.example`, `.env.sample`, `.env.template`, `.env.dist`. The number of skipped files is reported.
- **WSL symbolic links:** links created inside WSL hold absolute paths such as `/mnt/c/Users/…` (or `/mnt/host/c/…`), which do not resolve in the helper. After copying, each such link whose target lies under the mounted folder is replaced by a copy of that target (without `.env` files); other broken links are left as they are, and files that cannot be read never make an inspection or a copy fail.

The file list is built first and then filtered with `grep -z`, then passed to `tar --files-from`. (An earlier version used tar's `--exclude`, which GNU tar ignored because of argument order.)

The result goes to `/staging/<comparison id>/project` in the volume `ai-compare_staging` (or `/staging/<comparison id>/hidden` for hidden tests), and the script prints one JSON line: `{"mode":"git","files":76,"kilobytes":412,"envFilesSkipped":1}`.

The copy happens **once per comparison** and both sides build from it. Comparisons without a project skip the helper: `api` just creates an empty `/staging/<id>/project`. Agents never work on the shared host folder, which is also much faster on macOS and Windows, where shared folders are slow.

## The folder browser

Browsers never reveal a folder's absolute path (`showDirectoryPicker` and `<input webkitdirectory>` give access to the contents only), and the copy needs the path. So the **Browse…** dialog is the app's own: `ProjectService.ListFolders` runs `list-folders.sh` in the helper with the same read-only mount of the top-level folder, and returns the sub-folders (hidden ones and symbolic links left out, Git repositories flagged).

- It starts at the user's home folder on the host, which Compose passes to `api` as `HOST_HOME` (from `USERPROFILE` on Windows, `HOME` elsewhere).
- `parent` is empty at the top-level folder (`C:\`, `/Users`, `/home`), because nothing above it can be mounted.
- Host paths are rebuilt in the host's style (`joinHostPath`, the inverse of `splitHostPath`).
- Each listing starts a short-lived container: about 0.4 s.
- Only folders Docker can see are browsable (on Windows, the drives Docker Desktop shares). Typing a path still works.

## Hidden tests

The profile can name a host folder of **hidden tests**. Right after the project copy, `copy-project.sh` copies it with the same rules (read-only mount, `.env` files left out) to `/staging/<id>/hidden`, next to the project copy but **not** in the side images, so the agents never see it. It is used only by the second test run of [verification](#after-the-agent-result-image-collection-and-tests). If the copy fails, the comparison carries on without hidden tests and both sides log a warning.

## The side image

`BuildSideImage` builds one image per side from a tar context that `api` assembles from the staging volume (mounted in `api` at `/data/staging`).

Generated Dockerfile, layer by layer:

```dockerfile
FROM node:22-bookworm-slim                       # the profile's runtime
RUN apt-get update && apt-get install -y --no-install-recommends git ca-certificates && rm -rf /var/lib/apt/lists/*
RUN useradd -m -s /bin/sh agent                  # the unprivileged user the agent runs as
RUN npm install -g opencode-ai@1.18.34 && npm cache clean --force   # the CLI
WORKDIR /workspace
COPY project/ /workspace/                        # the copy
RUN npm ci                                       # the profile's setup, when set (as root)
RUN git init -q -b baseline && git config core.autocrlf false && \
    git config user.name ai-compare && git config user.email ai-compare@localhost && \
    git add -A && git commit -q --allow-empty -m baseline && \
    chown -R agent:agent /workspace
COPY --chown=agent:agent home/ /home/agent/      # CLI configuration (opencode.json)
USER agent
ENV HOME=/home/agent
```

Why this order:

- **Runtime, Git, the user and the CLI come before the project**, so those layers are cached across all comparisons and projects. Only the first comparison pays for installing opencode (about 55 s); later builds take a few seconds.
- **The baseline commit comes after setup**, so dependencies installed by setup (if not ignored) are part of the baseline and the diff shows only what the agent changed.
- **`core.autocrlf=false`** keeps Windows line endings exactly as copied, so they do not appear as changes.
- **`/workspace` is handed to `agent` after the baseline**, so the agent can change every file but the setup ran with root's permissions.
- **The CLI configuration comes after the baseline**, so it never appears in the diff. It contains the proxy URL and model choice, not secrets.
- `--allow-empty` and an always-present `project/` folder in the context make empty projects work (an agent can start a project from scratch).

The build uses Docker's **classic builder** (`BuilderV1`) from a tar stream. It works with Docker 29 and needs no BuildKit session handling in Go. The builder's JSON output is parsed; on failure the last lines are shown in the side's log.

Images are tagged `ai-compare/side:<id>-<side>` and labelled with the comparison, the side and `ai-compare.role=side`.

## After the agent: result image, collection and tests

When a side's agent stops, `api` turns what it left into artefacts and runs the tests, all in **short-lived containers without network** (`NetworkMode: none`) that are removed when they finish. Each carries the comparison and side labels and its own role.

| Step | Image | User | Mounts | Role | Output |
|---|---|---|---|---|---|
| **Commit** (`CommitResult`) | the stopped agent container becomes `ai-compare/result:<id>-<side>` | | | `result` (label on the image) | The image |
| **Collect** (`CollectResult`, `collect-result.sh`) | result image | root | artefacts volume at `/artifacts` | `collect` | `workspace.tar`, `solution.diff`/`.numstat`, `harness.diff`/`.numstat`, `dependencies.count`, `session.json` in `/artifacts/<id>/<side>` |
| **Tests** (`RunTests`) | result image | `agent` | none | `test` | Exit code and output (saved by `api` as `tests-visible.log`) |
| **Hidden tests** (`RunTests` with hidden) | result image | `agent` | staging volume at `/staging`, read-only | `test-hidden` | Same, after `cp -R /staging/<id>/hidden/. /workspace/` (`tests-hidden.log`) |

- **Why commit an image.** The result can be inspected and tested in containers that start from exactly what the agent left, without touching (or restarting) the agent's container, and it outlives that container.
- **Why the tests run in a fresh container.** Nothing the agent left running (servers, watchers, changed environment) affects them, and the hidden tests are never visible to the agent.
- **Why without network.** The tests cannot reach the internet, the proxy or anything else; a test suite that needs network fails (see [Status and roadmap](18-status-and-roadmap.md#known-limitations)).
- **`collect-result.sh`** runs as root because it reads a repository owned by `agent`; it sets `safe.directory=*` through `GIT_CONFIG_*` variables, so both Git and opencode (which runs Git itself to find its project) accept it. It exports opencode's sessions with `HOME=/home/agent`, where the agent kept them. The diffs use `git add -A` and `git diff --cached baseline`, with harness files at the root (`AGENTS.md`, `CLAUDE.md`, `.claude`, `.opencode`, `opencode.json`, `.mcp.json`…) excluded from the solution diff and alone in the harness diff. Dependency folders (`node_modules`, `.venv`…) are excluded too and only counted.
- Collection has a 5-minute timeout, each test run 10 minutes.

**Live diff.** While the side runs, `LiveDiff` uses `docker exec` in the agent container itself: it reads `baseline` into a temporary index (`GIT_INDEX_FILE=/tmp/ai-compare-index`), stages everything there and diffs it, so the agent's own Git index is untouched.

## Clean-up

`RemoveDockerObjects(id)` removes, for one comparison:

- every container labelled `ai-compare.comparison=<id>` (agents and any leftover helpers);
- the images `ai-compare/side:<id>-a`, `-b` and `ai-compare/result:<id>-a`, `-b`;
- the staging folder `<id>` (project copy and hidden tests).

It selects only by those labels and names, so nothing else on the user's Docker is touched. Retention calls `Remove` (the same, limited to what the retention setting selects, artefacts included when chosen) for comparisons that ended more than the configured number of days ago; deleting a comparison calls it and also removes its artefacts (`RemoveArtifacts`). See [Comparison lifecycle](04-comparison-lifecycle.md#retention-and-deletion).

`DiskUsage` reports, for the Settings page, the size of images labelled `ai-compare.role` (each image's own layers plus the largest shared part once, so the runtime and CLI layers are not counted per side), of the artefacts folder and of the staging folder.

## Presets

A preset is a reusable set of harness files (package `internal/presets`, served by `PresetService`); [Harnesses and presets](19-harnesses-and-presets.md) is the full picture. It lives in the data volume (`ai-compare_appdata`, `/data/app` in `api`) as `harnesses/<slug>/`:

```
harnesses/strict-backend/
  preset.md      frontmatter (title, description, clis) and free notes
  project/       a literal mirror of what goes to the project root (AGENTS.md, .claude/…)
  home/          what goes to the agent's home folder (e.g. .codex/config.toml)
```

- **Files** are written through the API (the editor, files and folders dropped in the browser) or imported from a host project: `copy-paths.sh` in the copy helper copies the chosen paths, read-only and without `.env` files, to a temporary folder in staging, which `api` then adds to the preset. The usual harness files are preselected; any other entry at the project root can be picked too (`InspectProject` lists them as `otherEntries`), for instructions kept in folders such as `rules/` or `skills/`. What CLIs keep for themselves is never imported: Claude Code's and Codex's worktrees (whole copies of the project), `node_modules`, `.git`, lock files and Codex sessions and logs. Paths are confined to the preset; values that look like API keys produce a warning (secrets belong in `.env`).
- **Hash:** a short hash of every file's path and content. When a comparison starts, each side using a preset records its title and hash and gets its own copy of the preset (in staging for the build, and in its artefacts), so editing or deleting the preset later does not change what explains an old result.
- **In the side image** (`BuildSideImage`): with a preset or **No harness**, every project path that is a harness file or inside a harness folder (`AGENTS.md`, `CLAUDE.md`, `.claude/`, `.opencode/`, `opencode.json`, `.mcp.json`, `.github/copilot-instructions.md`…, at any depth) is left out of the build context; a preset's `project/` files are added on top and its `home/` files go to the agent's home folder; the CLI's own settings go to a root-owned system file (opencode's managed configuration) that nothing in a harness can override. Plugin dependencies declared in `.opencode/package.json` or `~/.config/opencode/package.json` are installed with `npm install` while the image is built. All of this happens before the baseline commit, so the diff shows only what the agent changed.

## What is not done yet
- Runtimes without Node.js (for example `python:3.12`) need the CLI installed differently; planned with the codex and claude adapters.
