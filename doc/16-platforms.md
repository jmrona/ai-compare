# 16. Platforms

macOS is the primary platform; Windows and Linux must work too. The only host requirement is Docker with Compose. Everything else runs in containers, and every host-side command is a pnpm script or a `docker compose` command (no bash required).

| Topic | macOS | Windows | Linux | How it is handled |
|---|---|---|---|---|
| Docker socket | Docker Desktop, OrbStack: `/var/run/docker.sock`. Colima: `~/.colima/default/docker.sock` | `/var/run/docker.sock` (Docker Desktop) | `/var/run/docker.sock` | `DOCKER_SOCKET` in `.env`, default `/var/run/docker.sock` |
| Project paths | `/Users/…` is shared with Docker by default; `/Volumes/…` and others must be added in Docker Desktop's file sharing | `C:\…`; Docker Desktop translates it | Native | The copy helper mounts the top-level folder (`/Users`, `C:\`, `/home`) read-only and checks the path inside; unshared folders produce an explanatory error |
| Missing paths | An error | Docker Desktop **creates** missing bind sources | An error | Never bind-mount the project folder itself, only its always-existing top-level folder |
| CPU architecture | Apple Silicon (arm64) | amd64 | amd64 or arm64 | Multi-arch base images; nothing pins `amd64` |
| `host.docker.internal` | Built in | Built in | Not by default | `extra_hosts: host.docker.internal:host-gateway` on `api` |
| Shared-folder speed | Slower than native disk | Slower | Native | The project is copied once to a volume; agents never work on the shared folder |
| Line endings | LF | The repo forces LF (`.gitattributes`); user projects are copied as they are | LF | `core.autocrlf=false` in each side's baseline; embedded scripts normalised to LF |
| Permissions on the mounted project | Fine | Fine | The container user may lack read access | The copy helper runs as root with a read-only mount; sides work on the copy |
| Files created by `pnpm gen` | Owned by the user | Owned by the user | Owned by root (container user) | Known; `chown` them if needed |

## Verification status

| Platform | State |
|---|---|
| Windows 11 + Docker Desktop | All phase 0 points verified; phase 1 verified end to end with real OpenAI runs (verification with hidden tests, report, replay, restart with reattachment, deleting a comparison) |
| macOS | **Pending manual check:** copy from `/Users`, the "not shared" error (for example a path under `/Volumes`), terminals (typing, resizing, Ctrl+C) |
| Linux | Not checked by hand yet; the plan is CI on Linux with real Docker |

Planned testing: CI on Linux with real Docker, Go and frontend tests; on macOS and Windows runners only Go and frontend tests (their CI runners have no usable Docker); Docker on macOS and Windows checked by hand before each release.

## Git Bash on Windows

Git Bash rewrites arguments that look like Unix paths. Commands such as `docker compose exec api /app/spike …` need `MSYS_NO_PATHCONV=1` in front, or they try to run `C:/Program Files/Git/app/spike`. PowerShell and cmd are not affected.
