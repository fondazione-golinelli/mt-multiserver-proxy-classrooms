# Classrooms Plugin — Progress Tracker

## Status: All Phases Complete — Ready for Testing

### Phase Overview

| Phase | Description | Status |
|-------|-------------|--------|
| 1 | Scaffold + Data Model | **Complete** |
| 2 | Instance Lifecycle | **Complete** |
| 3 | Access Control | **Complete** |
| 4 | Chat Commands | **Complete** |
| 5 | Teacher Formspecs | **Complete** |
| 6 | Admin Formspecs | **Complete** |
| 7 | Mod Channel + Join/Leave | **Complete** |
| 8 | Bridge Lua Mod | **Complete** |

---

### Phase 1: Scaffold + Data Model

- [x] `go.mod` — module init with proxy + mysql driver dependencies
- [x] `main.go` — unified config loader, controller struct, `init()`
- [x] `db.go` — MySQL connection pool, auto-migration (CREATE TABLE IF NOT EXISTS)
- [x] `state.go` — CRUD methods backed by MySQL (teachers, classes, students, instances, invites, runtime state)
- [x] `pelican.go` — Pelican/Wings API client (create, fetch, mount, reinstall, delete, start/stop/kill daemon)
- [x] Verify: `go build -buildmode=plugin` compiles cleanly

### Phase 2: Instance Lifecycle

- [x] `instance.go` — provisionInstance, startInstance, stopInstance, deleteInstance
- [x] Startup reconciliation (re-register running instances)
- [x] Background status check goroutine

### Phase 3: Access Control

- [x] `access.go` — isAllowedToHop() logic
- [x] Intercept /join and >join commands

### Phase 4: Chat Commands

- [x] `commands.go` — >classes, >admin, >freeze/unfreeze, >teacher_add/remove/list, >lobby
- [x] Slash command interceptors (/classes, /admin, /freeze, /lobby)

### Phase 5: Teacher Formspecs

- [x] `formspec.go` — port existing screens (main, class, students, tp_selector)
- [x] `formspec.go` — add classrooms:create_instance (template picker)
- [x] `formspec.go` — add classrooms:instance (instance detail view)
- [x] `handlers.go` — port existing handlers
- [x] `handlers.go` — add instance creation/management handlers
- [x] Class view extended with INSTANCES section

### Phase 6: Admin Formspecs

- [x] classrooms:admin panel (all instances, teachers, templates)
- [x] classrooms:admin_create (create from any template)

### Phase 7: Mod Channel + Join/Leave

- [x] `modchan.go` — merge both plugins' channel handling
- [x] Join/leave hooks (re-apply freeze, ensure channel join)

### Phase 8: Luanti Bridge Mod (`classrooms_bridge`)

- [x] `classrooms_bridge/mod.conf`
- [x] `classrooms_bridge/init.lua` — mod channel listener + action handlers (freeze/unfreeze/watch/gather/tp/broadcast/ping)
- [x] Update mod channel name to `classrooms:cmd`
- [x] Smoother watching loop (globalstep)

---

## Decision Log

| Date | Decision | Rationale |
|------|----------|-----------|
| 2026-04-15 | Merge pelican-bridge + teachertools into single plugin | Go plugins can't import each other |
| 2026-04-15 | Flat teacher model (no school hierarchy) | Simpler; teachers own classes, admins see all |
| 2026-04-15 | Persistent instances | Teachers can resume multi-day projects |
| 2026-04-15 | Pre-assigned student usernames | Teacher adds names before students register |
| 2026-04-15 | Public/non-public templates | Non-public = admin-only dev templates |
| 2026-04-15 | Unlimited instances (with future limits) | Encourages collaboration |
| 2026-04-15 | Formspec UI for everything | Teachers and admins manage from within Luanti |
| 2026-04-15 | Class members auto-join, others by invite | Access control at hop level |
| 2026-04-15 | New git repo for classrooms plugin | github.com/fondazione-golinelli/mt-multiserver-proxy-classrooms |
| 2026-04-15 | Use UUID:port for backend address | Works with Docker container networking where UUID is hostname |
| 2026-04-15 | Re-apply state in onChatMsg(join) | Workaround for proxy lack of post-hop hook |
