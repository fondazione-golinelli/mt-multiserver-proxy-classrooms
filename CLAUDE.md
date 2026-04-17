# Classrooms Plugin for mt-multiserver-proxy

## What This Is

A Go plugin (`.so`) for [mt-multiserver-proxy](https://github.com/HimbeerserverDE/mt-multiserver-proxy) that combines classroom management with Pelican instance provisioning. Teachers manage classes and students via in-game formspec UI, provision persistent Luanti server instances from Pelican templates, and control students (freeze/watch/gather/teleport) across servers. Admins get a global panel.

## Status

All 8 implementation phases complete — plugin compiles (`.so` present), schema is auto-migrated on startup, reconciliation re-registers running instances with the proxy. See [PROGRESS.md](PROGRESS.md) for the phase-by-phase record and [PLAN.md](PLAN.md) for the original design.

## Architecture

```
Luanti clients
    |
mt-multiserver-proxy (lobby = default server)
    |-- classrooms.so (this plugin)
    |       |- MySQL-backed state (teachers, classes, students, instances, invites)
    |       |- Pelican Application + Wings daemon HTTP client
    |       |- formspec UI (teacher + admin)
    |       |- onChatMsg interceptor for /join, /classes, /admin, /lobby, /freeze, /unfreeze
    |       |- mod channel "classrooms:cmd" (freeze/watch/gather/tp_to actions)
    |       |- startup reconcile + 60s status checker
    |
    +-- lobby server (static, always running)
    +-- instance-A, instance-B, ... (dynamic, Pelican-managed)
```

## Key Files

| File | Responsibility |
|------|---------------|
| [main.go](main.go) | Plugin init, config loader (`classroomsConfig`), controller struct, runtime state, `isTeacher`/`isAdmin`, op-pending tracking |
| [db.go](db.go) | MySQL connection pool (`openDB`), auto-migration (`migrateDB`) |
| [state.go](state.go) | CRUD for teachers, classes, students, instances, invites; runtime freeze/watch/active-class maps; `gatherClass`, `teleportToPlayer`, `watchSingleStudent` |
| [pelican.go](pelican.go) | Pelican Application API (create/fetch/mount/reinstall/delete) + Wings daemon API (start/stop/kill/state); `nodeEndpointFor` cache |
| [instance.go](instance.go) | `provisionInstance`, `startInstance`, `stopInstance`, `deleteInstance`, `evacuateToLobby`, `reconcileInstances`, `startStatusChecker` |
| [commands.go](commands.go) | `>classes`, `>admin`, `>lobby`, `>freeze`, `>unfreeze`, `>teacher_add`, `>teacher_remove`, `>teacher_list` |
| [access.go](access.go) | `isAllowedToHop` + `onChatMsg` interceptor for `/join`/`>join` and slash-form aliases of the chat commands |
| [formspec.go](formspec.go) | Builders for `classrooms:main`, `classrooms:class`, `classrooms:template_picker`, `classrooms:instance`, `classrooms:admin`, `classrooms:students` |
| [handlers.go](handlers.go) | `RegisterOnPlayerReceiveFields` handlers for each formspec |
| [modchan.go](modchan.go) | `classrooms:cmd` channel registration, `ensureChannelJoin`, join/leave hooks, `reapplyStates` after hop |

## Capabilities

### Teacher (anyone in the `teachers` table, or holding `server` perm)
- `>classes` / `/classes` → dashboard with class list + create-class field
- Per-class view: freeze/unfreeze all, watch-me, stop watching, gather all, edit student names, per-student TP and single-student watch
- Provision a new instance from any **public** template, bound to the active class
- Instance view: start/stop/delete, "Hop Me Here", "Hop Class Here", invite by name
- `>freeze <name|class>` / `>unfreeze <name|class>` — by player OR class name
- `>lobby` / `/lobby` — hop back to the configured lobby server

### Admin (proxy `server` perm)
- Everything teachers can do, plus:
- `>admin` / `/admin` → global panel: all instances (any status ≠ `deleted`), teacher add/remove
- Template picker shows **all** templates including non-public ones
- Teacher management: `>teacher_add`, `>teacher_remove`, `>teacher_list`

### Instance Lifecycle
1. `provisionInstance`: create Pelican server → wait for initial install → attach mount(s) → reinstall → wait → start daemon → `proxy.AddServer`
2. On proxy startup: `reconcileInstances` re-registers every DB instance with status `running`
3. Every 60s: `checkAllInstancesStatus` polls Wings; instances that are no longer `running`/`starting` get marked `stopped` and removed from the proxy
4. `stopInstance` evacuates players to lobby, `RmServer`s, then stops daemon (falls back to kill on timeout)
5. `deleteInstance` evacuates, stops if running, deletes the Pelican server, marks row `deleted`

### Access Control
`isAllowedToHop(player, serverName)` — called from the `/join`/`>join` interceptor. Allows if the target is a static server (no instance row), creator, admin, class member (when instance has `class_id`), or has an `instance_invites` row. Formspec-triggered hops (`btn_hop_me`, `btn_hop_class`) go through plugin code already, so they don't need the same gate. **Known limitation**: another plugin calling `cc.Hop()` bypasses this.

### Mod Channel (`classrooms:cmd`)
JSON payloads sent to the player's current backend server. Actions emitted:
- `freeze` / `unfreeze` (with `player`)
- `watch` / `unwatch` (with `player` + `teacher`)
- `gather` (with `players` array + `target`)
- `tp_to` (with `player` + `target`)

The Luanti-side bridge mod must implement matching handlers — see the [Luanti-Side Bridge Mod](#luanti-side-bridge-mod) section.

### Join/Leave Hooks
- On join: goroutine calls `ensureChannelJoin` (retries up to 12× to join `classrooms:cmd`), then `reapplyStates` after 2s — re-sends freeze/watch so the state survives hop/reconnect.
- On leave: clears that player's `activeClass`, `activeInstance`, and `watchingPlayers` entry (for both students and teachers — if a teacher leaves, all their watched students are cleared).

## Config

Single JSON at `plugins/classrooms/config.json` (or `$CLASSROOMS_CONFIG`, or `plugins/classrooms.json`, or `classrooms.json`). `DisallowUnknownFields` is strict. See [config.example.json](config.example.json).

**Required**: `panel_url`, `application_api_token` (or `_file`), `default_game`, `lobby_server`, `db_host`, `db_name`, `db_user`, at least one `templates` entry.

**Template fields**: `user_id`, `egg_id`, `mount_id` (or `mount_ids`), `template_name`, `location_ids`, `media_pool` are required. Many defaults are filled in by `validateTemplate` (e.g. `internal_port=30000`, `world_name="world"`, `instance_template_mount="/home/mount"`). `public: true` makes a template visible to non-admin teachers.

**Timing defaults** (all overridable): `poll_interval_seconds=2`, `poll_timeout_seconds=180`, `start_grace_seconds=2`, `join_retry_count=12`, `join_retry_delay_millis=1000`.

## Database

MySQL via `database/sql` + `github.com/go-sql-driver/mysql`. Tables auto-created on startup (`migrateDB`).

- **Host**: Pelican-provisioned MySQL (accessible as `pelican-userdb:3306` from within Pelican containers — see memory `reference_classrooms_db.md`)
- **Tables**: `teachers`, `classes`, `class_students`, `instances`, `instance_invites`
- **Instance status values**: `provisioning` | `running` | `stopped` | `deleted` (deletes are soft — the row stays with `status='deleted'`)
- **Runtime-only state** (freeze, watch, active-class, active-instance, pending-ops) is NOT in the database — kept on `controller.runtime`, protected by `controller.mu`

## Building

Plugin must be compiled against the exact same Go version and proxy source as the running proxy binary. Use the build script from the parent directory:

```bash
cd /home/docker/compose/pelican/development/mt-multiserver
./build-plugins.sh mt-multiserver-proxy-classrooms
```

Or manually:
```bash
go mod edit -replace=github.com/HimbeerserverDE/mt-multiserver-proxy=../../source
go mod tidy
go build -buildmode=plugin -o mt-multiserver-proxy-classrooms.so .
go mod edit -dropreplace=github.com/HimbeerserverDE/mt-multiserver-proxy
```

## Proxy API Patterns Used

- `proxy.RegisterChatCmd(...)` — `>command` handlers ([commands.go](commands.go))
- `proxy.RegisterOnChatMsg(...)` — `/join`/`>join` access control + `/classes` etc. slash aliases
- `proxy.RegisterOnPlayerReceiveFields(formname, ...)` — formspec submit handlers (exact-match formname)
- `proxy.RegisterOnJoin` / `proxy.RegisterOnLeave` — connect/disconnect hooks
- `proxy.RegisterOnSrvModChanMsg(...)` — channel gating (currently just filters `classrooms:cmd` — no inbound dispatch logic yet)
- `proxy.Find(name)`, `proxy.Players()`, `proxy.Clts()` — lookup
- `proxy.AddServer`, `proxy.RmServer` — dynamic backend registration
- `cc.Hop`, `cc.ShowFormspec`, `cc.SendChatMsg`, `cc.SendModChanMsg`, `cc.JoinModChan`, `cc.IsModChanJoined`, `cc.ServerName`, `cc.HasPerms`
- `proxy.FormspecEscape` — escape all user-supplied text

## Formspec Notes

- All use `formspec_version[6]` with real coordinates
- Formnames: `classrooms:main`, `classrooms:class`, `classrooms:template_picker`, `classrooms:instance`, `classrooms:admin`, `classrooms:students` (exact match — proxy does not support prefixes)
- Dynamic context (which class/instance is open) is tracked in `runtime.activeClass[player]` / `runtime.activeInstance[player]`, not encoded in the formname
- Color constants in [formspec.go:13-22](formspec.go#L13-L22): `headerColor`, `accent`, `light`, `muted`, `panel`, `success`, `danger`, `warning`
- Helpers: `fmtEsc`, `mcColorize`, `btn`, `btnExit`, `coloredLbl`, `box`

## Related Components

- **Proxy source**: `../../source/` (fork of HimbeerserverDE/mt-multiserver-proxy)
- **Pelican bridge (predecessor)**: `../mt-multiserver-proxy-pelican-bridge/` — superseded
- **Teachertools (predecessor)**: `../mt-multiserver-proxy-teachertools/` — superseded
- **Pelican panel**: `https://pelican.silvaserv.it`
- **GitHub**: `https://github.com/fondazione-golinelli/mt-multiserver-proxy-classrooms`

### Luanti-Side Bridge Mod

The proxy plugin sends JSON actions over mod channel `classrooms:cmd`. A Luanti mod on each backend server must receive and execute them. The bridge mod lives outside this repo:

- **Location**: `/home/docker/compose/pelican/development/luanti/plugins/classrooms_bridge/`
- **Predecessors** (same directory):
  - `pelican_multiserver_bridge/` — thin chat commands for minigame create/join (superseded by formspec UI)
  - `teacher_tools_bridge/` — receives freeze/watch/gather/tp/broadcast commands (core logic reused)
- **Mod channel**: `classrooms:cmd` (must match `modChannel` constant in [main.go:21](main.go#L21))
- **Pattern**: proxy sends `{"action":"freeze","player":"X"}`, mod dispatches to a handler

As the proxy plugin gains new capabilities, the bridge mod needs matching action handlers. Keep them in sync.
