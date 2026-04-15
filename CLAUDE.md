# Classrooms Plugin for mt-multiserver-proxy

## What This Is

A Go plugin (`.so`) for [mt-multiserver-proxy](https://github.com/HimbeerserverDE/mt-multiserver-proxy) that combines classroom management with Pelican instance provisioning. Teachers manage classes and students via in-game formspec UI, create persistent Luanti server instances from templates, and control students (freeze/gather/teleport) across servers.

## Architecture

```
Luanti clients
    |
mt-multiserver-proxy (lobby = default server)
    |-- classrooms.so (this plugin)
    |       |- teacher/class/student state (JSON persistence)
    |       |- Pelican API client (provision/start/stop/delete instances)
    |       |- formspec UI (teacher panel + admin panel)
    |       |- access control (who can hop to which instance)
    |       |- mod channel bridge (freeze/gather/tp commands)
    |
    +-- lobby server (static, always running)
    +-- instance-A (dynamic, Pelican-managed)
    +-- instance-B (dynamic, Pelican-managed)
    ...
```

## Key Files

| File | Responsibility |
|------|---------------|
| `main.go` | Plugin init, unified config, controller struct |
| `db.go` | MySQL connection, auto-migration, query helpers |
| `state.go` | CRUD methods for teachers, classes, students, instances (uses db.go) |
| `pelican.go` | Pelican panel + Wings daemon HTTP API client |
| `instance.go` | Instance lifecycle: provision, start, stop, delete, reconcile |
| `commands.go` | Chat commands (`>classes`, `>admin`, `>freeze`, etc.) |
| `formspec.go` | All formspec screen builders |
| `handlers.go` | All formspec field handlers |
| `modchan.go` | Mod channel communication + join/leave hooks |
| `access.go` | Hop access control enforcement |

## Building

The plugin must be compiled against the exact same Go version and proxy source as the running proxy binary. Use the build script from the parent directory:

```bash
cd /home/docker/compose/pelican/development/mt-multiserver
./build-plugins.sh mt-multiserver-proxy-classrooms
```

Or manually:
```bash
go mod edit -replace=github.com/HimbeerserverDE/mt-multiserver-proxy=../../source
go mod tidy
go build -buildmode=plugin -o classrooms.so .
go mod edit -dropreplace=github.com/HimbeerserverDE/mt-multiserver-proxy
```

## Proxy API Patterns

The proxy provides plugin hooks via package-level functions:
- `proxy.RegisterChatCmd(...)` — register `>command` handlers
- `proxy.RegisterOnChatMsg(...)` — intercept chat (used for `/slash` commands and access control)
- `proxy.RegisterOnPlayerReceiveFields(formname, ...)` — formspec submit handlers (exact match on formname)
- `proxy.RegisterOnJoin(...)` / `proxy.RegisterOnLeave(...)` — player connect/disconnect
- `proxy.RegisterOnSrvModChanMsg(...)` — messages from backend server mods
- `proxy.Find(name)` — get `*ClientConn` for a player
- `proxy.Players()` — all connected players
- `cc.Hop(serverName)` — switch a player to another backend server
- `cc.ShowFormspec(name, spec)` — show a formspec to a player
- `cc.SendChatMsg(...)` — send chat message to a player
- `cc.SendModChanMsg(channel, msg)` — send mod channel message to player's current server
- `cc.ServerName()` — which backend server the player is on
- `cc.HasPerms(perm)` — check proxy-level permissions
- `proxy.AddServer(name, Server{...})` — dynamically register a backend server
- `proxy.FormspecEscape(s)` — escape text for formspec strings

## Formspec Notes

- All formspecs use `formspec_version[4]` (real coordinates)
- Formname prefix: `classrooms:` (e.g., `classrooms:main`, `classrooms:class`)
- Dynamic data (which class is open) is tracked in runtime state (`activeClass` map), NOT encoded in the formname — the proxy only supports exact formname matching
- Use `proxy.FormspecEscape()` for all user-supplied text in formspecs
- Color scheme constants: `headerColor`, `accent`, `light`, `muted`, `panel`

## Config

Single JSON file at `plugins/classrooms/config.json`. Combines Pelican API settings, MySQL connection details, and template definitions. Templates have a `public` bool — non-public templates are admin-only.

## Database

MySQL via `database/sql` + `github.com/go-sql-driver/mysql`. Tables auto-created on startup.

- **Host**: Pelican-provisioned MySQL database (accessible as `userdb:3306` from within Pelican containers)
- **Tables**: `teachers`, `classes`, `class_students`, `instances`, `instance_invites`
- **Driver**: standard `database/sql` — no ORM
- **Runtime-only state** (freeze/watch maps) is NOT in the database — kept in-memory on the controller struct

## Related Components

- **Proxy source**: `../../source/` (fork of HimbeerserverDE/mt-multiserver-proxy)
- **Pelican bridge (predecessor)**: `../mt-multiserver-proxy-pelican-bridge/` — being superseded by this plugin
- **Teachertools (predecessor)**: `../mt-multiserver-proxy-teachertools/` — being superseded by this plugin
- **Pelican panel**: `https://pelican.silvaserv.it` — server management panel
- **GitHub**: `https://github.com/fondazione-golinelli/mt-multiserver-proxy-classrooms`

### Luanti-Side Bridge Mod

The proxy plugin sends commands over mod channel — a Luanti mod on each backend server receives and executes them. This mod lives separately from the proxy plugin:

- **Location**: `/home/docker/compose/pelican/development/luanti/plugins/classrooms_bridge/`
- **Predecessors** (same directory):
  - `pelican_multiserver_bridge/` — thin chat commands for minigame create/join (superseded by formspec UI)
  - `teacher_tools_bridge/` — receives freeze/watch/gather/tp/broadcast commands (core logic reused)
- **Mod channel**: `classrooms:cmd` (must match proxy plugin config)
- **Pattern**: proxy sends JSON `{"action":"freeze","player":"X"}`, mod dispatches to handler

As the proxy plugin gains new capabilities, the bridge mod needs matching action handlers. Keep them in sync.

## Implementation Plan

See [PLAN.md](PLAN.md) for the full implementation plan and [PROGRESS.md](PROGRESS.md) for current status.
