# 5. Project copy and side images

Code: `backend/internal/workspace/workspace.go` and the helper image in `backend/internal/workspace/copier/`.

## The problem

`api` runs in a Linux container. The user's project is on the host (`C:\Users\…` on Windows, `/Users/…` on macOS, `/home/…` on Linux). `api` cannot see that folder, and mounting every possible project folder into `api` in advance is impossible.

The solution relies on **Docker-out-of-Docker**: `api` talks to the host's Docker daemon through the mounted socket, and bind-mount paths are resolved **by the daemon, on the host**. So `api` can create a container that mounts `C:\` even though `api` itself has no `C:\`.

## The copy helper

A tiny Alpine image (`git`, `tar`, `grep`, `findutils`, `coreutils`, `sed`) with three scripts:

- `copy-project.sh` — copies the project into the staging volume;
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

The file list is built first and then filtered with `grep -z`, then passed to `tar --files-from`. (An earlier version used tar's `--exclude`, which GNU tar ignored because of argument order.)

The result goes to `/staging/<comparison id>/project` in the volume `ai-compare_staging`, and the script prints one JSON line: `{"mode":"git","files":76,"kilobytes":412,"envFilesSkipped":1}`.

The copy happens **once per comparison** and both sides build from it. Comparisons without a project skip the helper: `api` just creates an empty `/staging/<id>/project`. Agents never work on the shared host folder, which is also much faster on macOS and Windows, where shared folders are slow.

## The folder browser

Browsers never reveal a folder's absolute path (`showDirectoryPicker` and `<input webkitdirectory>` give access to the contents only), and the copy needs the path. So the **Browse…** dialog is the app's own: `ProjectService.ListFolders` runs `list-folders.sh` in the helper with the same read-only mount of the top-level folder, and returns the sub-folders (hidden ones and symbolic links left out, Git repositories flagged).

- It starts at the user's home folder on the host, which Compose passes to `api` as `HOST_HOME` (from `USERPROFILE` on Windows, `HOME` elsewhere).
- `parent` is empty at the top-level folder (`C:\`, `/Users`, `/home`), because nothing above it can be mounted.
- Host paths are rebuilt in the host's style (`joinHostPath`, the inverse of `splitHostPath`).
- Each listing starts a short-lived container: about 0.4 s.
- Only folders Docker can see are browsable (on Windows, the drives Docker Desktop shares). Typing a path still works.

## The side image

`BuildSideImage` builds one image per side from a tar context that `api` assembles from the staging volume (mounted in `api` at `/data/staging`).

Generated Dockerfile, layer by layer:

```dockerfile
FROM node:22-bookworm-slim                       # the profile's runtime
RUN apt-get update && apt-get install -y --no-install-recommends git ca-certificates && rm -rf /var/lib/apt/lists/*
RUN npm install -g opencode-ai@1.18.34 && npm cache clean --force   # the CLI
WORKDIR /workspace
COPY project/ /workspace/                        # the copy
RUN npm ci                                       # the profile's setup, when set
RUN git init -q -b baseline && git config core.autocrlf false && \
    git config user.name ai-compare && git config user.email ai-compare@localhost && \
    git add -A && git commit -q --allow-empty -m baseline
COPY home/ /root/                                # CLI configuration (opencode.json)
```

Why this order:

- **Runtime, Git and the CLI come before the project**, so those layers are cached across all comparisons and projects. Only the first comparison pays for installing opencode (about 55 s); later builds take a few seconds.
- **The baseline commit comes after setup**, so dependencies installed by setup (if not ignored) are part of the baseline and the diff shows only what the agent changed.
- **`core.autocrlf=false`** keeps Windows line endings exactly as copied, so they do not appear as changes.
- **The CLI configuration comes after the baseline**, so it never appears in the diff. It contains the proxy URL and model choice, not secrets.
- `--allow-empty` and an always-present `project/` folder in the context make empty projects work (an agent can start a project from scratch).

The build uses Docker's **classic builder** (`BuilderV1`) from a tar stream. It works with Docker 29 and needs no BuildKit session handling in Go. The builder's JSON output is parsed; on failure the last lines are shown in the side's log.

Images are tagged `ai-compare/side:<id>-<side>` and labelled with the comparison and side.

## What is not done yet

- The harness files are copied as they are (phase 1). Excluding them from the diff, and replacing them with presets, come later.
- Runtimes without Node.js (for example `python:3.12`) need the CLI installed differently; planned with the codex and claude adapters.
- Cleanup of old images, containers and staging folders (retention) is not implemented.
- Agents run as root inside their container. The container is the safety boundary; a non-root user is planned.
