# Classrooms Plugin — Progress Tracker

## Status: Phase 1 Complete — Ready for Phase 2

### Phase Overview

| Phase | Description | Status |
|-------|-------------|--------|
| 1 | Scaffold + Data Model | **Complete** |
| 2 | Instance Lifecycle | Not Started |
| 3 | Access Control | Not Started |
| 4 | Chat Commands | Not Started |
| 5 | Teacher Formspecs | Not Started |
| 6 | Admin Formspecs | Not Started |
| 7 | Mod Channel + Join/Leave | Not Started |
| 8 | Bridge Lua Mod | Not Started |

---

### Phase 1: Scaffold + Data Model

- [x] `go.mod` — module init with proxy + mysql driver dependencies
- [x] `main.go` — unified config loader, controller struct, `init()`
- [x] `db.go` — MySQL connection pool, auto-migration (CREATE TABLE IF NOT EXISTS)
- [x] `state.go` — CRUD methods backed by MySQL (teachers, classes, students, instances, invites, runtime state)
- [x] `pelican.go` — Pelican/Wings API client (create, fetch, mount, reinstall, delete, start/stop/kill daemon)
- [x] Verify: `go build -buildmode=plugin` compiles cleanly

### Phase 2: Instance Lifecycle

- [ ] `instance.go` — provisionInstance, startInstance, stopInstance, deleteInstance
- [ ] Startup reconciliation (re-register running instances)
- [ ] Background status check goroutine

### Phase 3: Access Control

- [ ] `access.go` — isAllowedToHop() logic
- [ ] Intercept /join and >join commands

### Phase 4: Chat Commands

- [ ] `commands.go` — >classes, >admin, >freeze/unfreeze, >teacher_add/remove/list, >lobby
- [ ] Slash command interceptors (/classes, /admin, /freeze, /lobby)

### Phase 5: Teacher Formspecs

- [ ] `formspec.go` — port existing screens (main, class, students, tp_selector)
- [ ] `formspec.go` — add classrooms:create_instance (template picker)
- [ ] `formspec.go` — add classrooms:instance (instance detail view)
- [ ] `handlers.go` — port existing handlers
- [ ] `handlers.go` — add instance creation/management handlers
- [ ] Class view extended with INSTANCES section

### Phase 6: Admin Formspecs

- [ ] classrooms:admin panel (all instances, teachers, templates)
- [ ] classrooms:admin_create (create from any template)

### Phase 7: Mod Channel + Join/Leave

- [ ] `modchan.go` — merge both plugins' channel handling
- [ ] Join/leave hooks (re-apply freeze, ensure channel join)

### Phase 8: Luanti Bridge Mod (`classrooms_bridge`)

Merges `pelican_multiserver_bridge` + `teacher_tools_bridge` into one Luanti mod.
Location: `/home/docker/compose/pelican/development/luanti/plugins/classrooms_bridge/`

- [ ] `classrooms_bridge/mod.conf`
- [ ] `classrooms_bridge/init.lua` — mod channel listener + action handlers (freeze/unfreeze/watch/gather/tp/broadcast)
- [ ] Update mod channel name to `classrooms:cmd`
- [ ] Keep in sync with proxy plugin as new actions are added

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
