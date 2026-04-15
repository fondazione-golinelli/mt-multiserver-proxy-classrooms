# Unified "Classrooms" Plugin — Implementation Plan

## Context

Two existing mt-multiserver-proxy plugins need to be merged into one:
- **pelican-bridge** — provisions Luanti instances via Pelican API
- **teachertools** — class/student management with formspec GUIs

The goal is a single plugin that lets teachers manage classes, provision persistent Luanti instances for those classes, control students (freeze/gather/teleport), and manage instance lifecycle — all from within Luanti's formspec UI. Admins get a separate panel for global management.

### Key Decisions
- **Single merged plugin** (Go plugins can't import each other)
- **Flat teacher model** — no school hierarchy; teachers own classes, admins see all
- **Persistent instances** — stay alive, teacher can resume/stop/delete manually
- **Student auth** — teacher pre-assigns usernames; when a user registers with that name, they're already in the class
- **Templates** — public (all teachers) and non-public (admin-only)
- **Unlimited concurrency** — multiple instances per class, with configurable limits later
- **In-game formspec UI** — for both teachers and admins
- **Access control** — class members auto-join instances, others need explicit invite

---

## File Structure

```
mt-multiserver-proxy-classrooms/
  go.mod
  main.go          -- init, unified config, controller struct
  db.go            -- MySQL connection, auto-migration, query helpers
  state.go         -- CRUD methods (teachers, classes, students, instances) using db.go
  pelican.go       -- Pelican/Wings API client (extracted from pelican-bridge)
  instance.go      -- instance lifecycle: provision, start, stop, delete, reconcile
  commands.go      -- chat commands (>classes, >admin, >freeze, >lobby, etc.)
  formspec.go      -- formspec builders (all screens)
  handlers.go      -- formspec field handlers
  modchan.go       -- mod channel + join/leave hooks
  access.go        -- hop access control
```

### Reuse Sources
- `pelican.go` <- extracted from `mt-multiserver-proxy-pelican-bridge/main.go` (lines 805-1065: all API methods, types)
- `state.go` <- rewritten: same CRUD interface as teachertools but backed by MySQL instead of JSON
- `db.go` <- new: MySQL connection pool, auto-migration, query helpers
- `formspec.go` <- extended from `mt-multiserver-proxy-teachertools/formspec.go` (add instance screens)
- `handlers.go` <- extended from `mt-multiserver-proxy-teachertools/handlers.go` (add instance handlers)
- `modchan.go` <- merged from both plugins' modchan/join-leave logic
- `commands.go` <- extended from teachertools' commands.go (drop >minigame, add >admin)

---

## Data Model

### Config (`classroomsConfig`)
Merges both plugin configs into one:
```go
type classroomsConfig struct {
    // Pelican API (from pelican-bridge)
    PanelURL, ApplicationToken, ApplicationTokenFile string
    PollIntervalSeconds, PollTimeoutSeconds          int
    StartGraceSeconds, JoinRetryCount, JoinRetryDelayMillis int

    // MySQL database
    DBHost     string // e.g. "userdb:3306"
    DBName     string
    DBUser     string
    DBPassword string
    DBPasswordFile string // alternative: read password from file

    // Templates — each has a new `Public bool` field
    Templates map[string]templateConfig
}
```

### Database Schema (MySQL, auto-migrated on startup)

```sql
CREATE TABLE teachers (
    username     VARCHAR(50) PRIMARY KEY,
    created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE classes (
    id           INT AUTO_INCREMENT PRIMARY KEY,
    name         VARCHAR(100) NOT NULL UNIQUE,
    created_by   VARCHAR(50) NOT NULL,
    created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (created_by) REFERENCES teachers(username)
);

CREATE TABLE class_students (
    class_id     INT NOT NULL,
    username     VARCHAR(50) NOT NULL,
    PRIMARY KEY (class_id, username),
    FOREIGN KEY (class_id) REFERENCES classes(id) ON DELETE CASCADE
);

CREATE TABLE instances (
    id            VARCHAR(100) PRIMARY KEY,  -- e.g. "math-alice-20260415-a3f2"
    class_id      INT,                       -- NULL = standalone admin instance
    created_by    VARCHAR(50) NOT NULL,
    template_name VARCHAR(100) NOT NULL,
    created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    server_id     INT NOT NULL,              -- Pelican server ID
    uuid          VARCHAR(36) NOT NULL,      -- Pelican server UUID
    node_id       INT NOT NULL,              -- Pelican node ID
    proxy_name    VARCHAR(200) NOT NULL,     -- registered in proxy
    backend_addr  VARCHAR(200) NOT NULL,     -- host:port
    status        VARCHAR(20) NOT NULL DEFAULT 'provisioning',
    FOREIGN KEY (class_id) REFERENCES classes(id) ON DELETE SET NULL
);

CREATE TABLE instance_invites (
    instance_id  VARCHAR(100) NOT NULL,
    username     VARCHAR(50) NOT NULL,
    PRIMARY KEY (instance_id, username),
    FOREIGN KEY (instance_id) REFERENCES instances(id) ON DELETE CASCADE
);
```

### Go Structs (mapped from DB rows)
```go
type classData struct {
    ID        int
    Name      string
    CreatedBy string
    CreatedAt time.Time
}

type instanceData struct {
    ID            string
    ClassID       *int     // nil = standalone
    CreatedBy     string
    TemplateName  string
    CreatedAt     time.Time
    ServerID      int
    UUID          string
    NodeID        int
    ProxyName     string
    BackendAddr   string
    Status        string   // "provisioning" | "running" | "stopped" | "deleted"
}
```

### Runtime State (not persisted, on controller struct)
```go
type runtimeState struct {
    frozenPlayers   map[string]bool    // player -> frozen
    watchingPlayers map[string]string  // student -> teacher
    activeClass     map[string]string  // teacher -> open class name
    activeInstance  map[string]string  // teacher -> open instance ID
    pendingOps      map[string]struct{} // player -> in-flight operation
}
```

All runtime maps protected by `controller.mu` (fixes existing race condition with package-level maps).

Driver: `database/sql` + `github.com/go-sql-driver/mysql`. Auto-migrate on startup (CREATE TABLE IF NOT EXISTS).

---

## Formspec Screen Flow

```
TEACHER:
  >classes / /classes
    |
    [classrooms:main] — Class list + create
    |
    +-- Open class --> [classrooms:class] — Student list, controls, INSTANCES section
    |       +-- Manage Students --> [classrooms:students]
    |       +-- TP to Student --> [classrooms:tp_selector]
    |       +-- Create Instance --> [classrooms:create_instance] — Template picker
    |       +-- Open Instance --> [classrooms:instance] — Instance detail/controls
    |
    +-- Close

ADMIN:
  >admin / /admin
    |
    [classrooms:admin] — All instances, all teachers, templates overview
    |
    +-- Open Instance --> [classrooms:instance]
    +-- Create Instance --> [classrooms:admin_create] — All templates (incl non-public)
    +-- Manage Teachers --> add/remove inline
```

### Instance View (`classrooms:instance`) — shared by teacher + admin
- Status indicator (running/stopped/provisioning)
- Start/Stop/Delete buttons
- Player list (who's currently on this server)
- "Hop Me Here" / "Hop Class Here" buttons
- Invite player (text field + add) / remove invite
- Back button

---

## Access Control

**Problem**: proxy has no `RegisterOnPreHop` hook.

**Solution**: Intercept all user-facing hop paths:
1. `/join`, `>join`, `>lobby` via `RegisterOnChatMsg` — check `isAllowedToHop()` before `cc.Hop()`
2. All formspec-triggered hops go through plugin code — already controlled
3. Mod channel hop requests — add check

```go
func isAllowedToHop(playerName, serverName string) bool {
    inst := findInstanceByProxyName(serverName)
    if inst == nil { return true } // static server (lobby etc)
    if inst.CreatedBy == playerName { return true }
    if isAdmin(playerName) { return true }
    if inst.ClassName != "" && isStudentInClass(inst.ClassName, playerName) { return true }
    if sliceContains(inst.InvitedPlayers, playerName) { return true }
    return false
}
```

**Known limitation**: another plugin calling `cc.Hop()` directly bypasses this. Acceptable for now; a future proxy patch could add a pre-hop hook.

---

## Implementation Phases

### Phase 1: Scaffold + Data Model
- Create `mt-multiserver-proxy-classrooms/` with `go.mod`
- `main.go` — unified config loader, `controller` struct, `init()` that registers everything
- `db.go` — MySQL connection pool, auto-migration (CREATE TABLE IF NOT EXISTS), prepared helpers
- `state.go` — all CRUD methods backed by MySQL (teachers, classes, students, instances, invites)
- `pelican.go` — extract all Pelican API code from pelican-bridge verbatim, add `stopDaemonServer`
- Verify: `go build -buildmode=plugin`

### Phase 2: Instance Lifecycle
- `instance.go` — `provisionInstance`, `startInstance`, `stopInstance`, `deleteInstance`
- Startup reconciliation: re-register running instances with `proxy.AddServer`
- Background goroutine: periodic Pelican status check (every 60s) for running instances

### Phase 3: Access Control
- `access.go` — `isAllowedToHop()` logic
- Intercept `/join`, `>join` in chat message handler

### Phase 4: Chat Commands
- `commands.go` — `>classes`, `>admin`, `>freeze/unfreeze`, `>teacher_add/remove/list`, `>lobby`
- Slash command interceptors via `RegisterOnChatMsg`

### Phase 5: Teacher Formspecs
- `formspec.go` — port all existing screens, add `classrooms:create_instance` and `classrooms:instance`
- `handlers.go` — port existing handlers, add instance creation/management handlers
- Class view extended with "INSTANCES" section showing bound instances

### Phase 6: Admin Formspecs
- `classrooms:admin` panel — all instances, teacher management, template list
- `classrooms:admin_create` — create from any template (incl non-public)

### Phase 7: Mod Channel + Join/Leave
- `modchan.go` — merge both plugins' logic, handle `teachertools:cmd` channel
- Join/leave hooks: re-apply freeze, track server, ensure channel join

### Phase 8: Luanti Bridge Mod (`classrooms_bridge`)

Merges the two existing Luanti-side mods into one:
- **`pelican_multiserver_bridge`** (predecessor) — thin chat commands for minigame create/join/list via mod channel. Replaced by formspec UI.
- **`teacher_tools_bridge`** (predecessor) — receives proxy commands, executes freeze/watch/gather/tp/broadcast. Core logic is reused.

The merged mod lives at `development/luanti/plugins/classrooms_bridge/` and:
- Listens on mod channel `classrooms:cmd` for JSON commands from the proxy plugin
- Executes server-local actions: freeze/unfreeze physics, watch/unwatch (forced look), gather, tp_to, broadcast
- Globalstep loop for watch-teacher forced look direction
- Re-applies freeze on player join
- Will grow as the proxy plugin gains capabilities (e.g., new actions get new handlers)

Files:
- `classrooms_bridge/mod.conf`
- `classrooms_bridge/init.lua`

The mod channel name should match what the proxy plugin uses (configurable, default `classrooms:cmd`).

---

## Verification

1. **Build**: `go build -buildmode=plugin -o classrooms.so` compiles
2. **Unit**: state CRUD operations, access control logic, config loading
3. **Integration flow**:
   - Start proxy with plugin -> connect as admin -> `>teacher_add admin`
   - `>classes` -> create class -> add student names
   - Open class -> Create Instance (pick template) -> wait for provision
   - Instance appears as "running" -> "Hop Me Here"
   - Connect as student -> auto-lands in lobby -> verify access control (can join instance if in class, blocked if not)
   - Freeze/gather/broadcast students across servers
   - Stop instance -> verify Wings stops
   - Resume instance -> verify Wings starts, proxy re-registers
   - Delete instance -> verify Pelican deletes, proxy unregisters
4. **Admin flow**: `>admin` -> see all instances, force-delete, create from non-public template
5. **Bridge mod**: install on backend server, verify freeze/unfreeze/gather commands work via mod channel
