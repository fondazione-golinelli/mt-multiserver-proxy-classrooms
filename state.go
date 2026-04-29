package main

import (
	"database/sql"
	"sort"
	"strings"
	"time"

	proxy "github.com/HimbeerserverDE/mt-multiserver-proxy"
)

// ── Go structs mapped from DB rows ─────────────────────────────────────────

type classData struct {
	ID        int
	Name      string
	CreatedBy string
	CreatedAt time.Time
}

type teacherData struct {
	Username  string
	Institute string
	CreatedAt time.Time
}

type instanceData struct {
	ID           string
	ClassID      *int // nil = standalone
	CreatedBy    string
	Institute    string
	DisplayName  string
	TemplateName string
	CreatedAt    time.Time
	ServerID     int
	UUID         string
	NodeID       int
	ProxyName    string
	BackendAddr  string
	Status       string // "provisioning" | "running" | "stopped" | "deleted"
}

func (inst instanceData) Title() string {
	if strings.TrimSpace(inst.DisplayName) != "" {
		return inst.DisplayName
	}
	return inst.ID
}

// ── Counts (for init log) ──────────────────────────────────────────────────

func (c *controller) countTeachers() (int, error) {
	var n int
	err := c.db.QueryRow("SELECT COUNT(*) FROM teachers").Scan(&n)
	return n, err
}

func (c *controller) countClasses() (int, error) {
	var n int
	err := c.db.QueryRow("SELECT COUNT(*) FROM classes").Scan(&n)
	return n, err
}

func (c *controller) countInstances() (int, error) {
	var n int
	err := c.db.QueryRow("SELECT COUNT(*) FROM instances WHERE status != 'deleted'").Scan(&n)
	return n, err
}

// ── Teacher management ─────────────────────────────────────────────────────

func (c *controller) getTeacher(name string) (bool, error) {
	var exists int
	err := c.db.QueryRow("SELECT 1 FROM teachers WHERE username = ?", name).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (c *controller) addTeacher(name string) error {
	_, err := c.db.Exec(
		"INSERT IGNORE INTO teachers (username) VALUES (?)", name)
	return err
}

func (c *controller) addTeacherWithInstitute(name, institute string) error {
	_, err := c.db.Exec(
		"INSERT INTO teachers (username, institute) VALUES (?, ?) ON DUPLICATE KEY UPDATE institute = VALUES(institute)",
		name, institute)
	return err
}

func (c *controller) removeTeacher(name string) error {
	_, err := c.db.Exec("DELETE FROM teachers WHERE username = ?", name)
	return err
}

func (c *controller) updateTeacherInstitute(name, institute string) error {
	_, err := c.db.Exec("UPDATE teachers SET institute = ? WHERE username = ?", institute, name)
	return err
}

func (c *controller) getTeacherInstitute(name string) (string, error) {
	var institute string
	err := c.db.QueryRow("SELECT institute FROM teachers WHERE username = ?", name).Scan(&institute)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return institute, err
}

func (c *controller) listTeacherRecords() ([]teacherData, error) {
	rows, err := c.db.Query("SELECT username, institute, created_at FROM teachers ORDER BY username")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []teacherData
	for rows.Next() {
		var td teacherData
		if err := rows.Scan(&td.Username, &td.Institute, &td.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, td)
	}
	return result, rows.Err()
}

func (c *controller) listTeachers() ([]string, error) {
	rows, err := c.db.Query("SELECT username FROM teachers ORDER BY username")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		result = append(result, name)
	}
	return result, rows.Err()
}

// ── Class management ───────────────────────────────────────────────────────

// getClasses returns classes visible to a teacher.
// Admins see all classes, teachers see only their own.
func (c *controller) getClasses(teacherName string) ([]classData, error) {
	cc := proxy.Find(teacherName)
	isAdmin := cc != nil && cc.HasPerms("server")

	var rows *sql.Rows
	var err error
	if isAdmin {
		rows, err = c.db.Query(
			"SELECT id, name, created_by, created_at FROM classes ORDER BY name")
	} else {
		rows, err = c.db.Query(
			"SELECT id, name, created_by, created_at FROM classes WHERE created_by = ? ORDER BY name",
			teacherName)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []classData
	for rows.Next() {
		var cd classData
		if err := rows.Scan(&cd.ID, &cd.Name, &cd.CreatedBy, &cd.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, cd)
	}
	return result, rows.Err()
}

func (c *controller) getAllClasses() ([]classData, error) {
	return c.getFilteredClasses("", "")
}

func (c *controller) getFilteredClasses(institute, teacher string) ([]classData, error) {
	query := "SELECT c.id, c.name, c.created_by, c.created_at FROM classes c LEFT JOIN teachers t ON t.username = c.created_by WHERE 1=1"
	var args []interface{}
	if institute != "" {
		query += " AND COALESCE(t.institute, '') = ?"
		args = append(args, institute)
	}
	if teacher != "" {
		query += " AND c.created_by = ?"
		args = append(args, teacher)
	}
	query += " ORDER BY c.name"

	rows, err := c.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []classData
	for rows.Next() {
		var cd classData
		if err := rows.Scan(&cd.ID, &cd.Name, &cd.CreatedBy, &cd.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, cd)
	}
	return result, rows.Err()
}

func (c *controller) getClassByID(id int) (*classData, error) {
	var cd classData
	err := c.db.QueryRow(
		"SELECT id, name, created_by, created_at FROM classes WHERE id = ?", id,
	).Scan(&cd.ID, &cd.Name, &cd.CreatedBy, &cd.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cd, nil
}

func (c *controller) getClassByName(name string) (*classData, error) {
	var cd classData
	err := c.db.QueryRow(
		"SELECT id, name, created_by, created_at FROM classes WHERE name = ?", name,
	).Scan(&cd.ID, &cd.Name, &cd.CreatedBy, &cd.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cd, nil
}

func (c *controller) createClass(teacherName, className string) (bool, string) {
	if className == "" || len(className) > 100 {
		return false, "Class name must be 1-100 characters."
	}

	existing, err := c.getClassByName(className)
	if err != nil {
		return false, "Database error."
	}
	if existing != nil {
		return false, "A class with that name already exists."
	}

	_, err = c.db.Exec(
		"INSERT INTO classes (name, created_by) VALUES (?, ?)",
		className, teacherName)
	if err != nil {
		return false, "Failed to create class."
	}
	return true, "Class created."
}

func (c *controller) deleteClass(teacherName string, classID int) (bool, string) {
	cls, err := c.getClassByID(classID)
	if err != nil || cls == nil {
		return false, "Class not found."
	}

	cc := proxy.Find(teacherName)
	isAdmin := cc != nil && cc.HasPerms("server")
	if cls.CreatedBy != teacherName && !isAdmin {
		return false, "You don't own this class."
	}

	_, err = c.db.Exec("DELETE FROM classes WHERE id = ?", classID)
	if err != nil {
		return false, "Failed to delete class."
	}
	return true, "Class deleted."
}

// ── Student management ─────────────────────────────────────────────────────

func (c *controller) getStudents(classID int) ([]string, error) {
	rows, err := c.db.Query(
		"SELECT username FROM class_students WHERE class_id = ? ORDER BY username",
		classID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		result = append(result, name)
	}
	return result, rows.Err()
}

func (c *controller) getOnlineStudents(classID int) []string {
	students, err := c.getStudents(classID)
	if err != nil {
		return nil
	}
	var online []string
	for _, name := range students {
		if proxy.Find(name) != nil {
			online = append(online, name)
		}
	}
	return online
}

func (c *controller) getStudentClass(studentName string) (*classData, error) {
	var cd classData
	err := c.db.QueryRow(`
		SELECT c.id, c.name, c.created_by, c.created_at
		FROM class_students cs
		JOIN classes c ON c.id = cs.class_id
		WHERE cs.username = ?`, studentName,
	).Scan(&cd.ID, &cd.Name, &cd.CreatedBy, &cd.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cd, nil
}

func (c *controller) addStudent(classID int, studentName string) (bool, string) {
	if studentName == "" {
		return false, "Student name is empty."
	}

	existing, err := c.getStudentClass(studentName)
	if err != nil {
		return false, "Database error."
	}
	if existing != nil {
		if existing.ID == classID {
			return false, studentName + " is already in this class."
		}
		return false, studentName + " is already assigned to class " + existing.Name + "."
	}

	_, err = c.db.Exec(
		"INSERT INTO class_students (class_id, username) VALUES (?, ?)",
		classID, studentName)
	if err != nil {
		return false, "Failed to add student."
	}
	return true, "Student added."
}

func (c *controller) removeStudent(classID int, studentName string) (bool, string) {
	res, err := c.db.Exec(
		"DELETE FROM class_students WHERE class_id = ? AND username = ?",
		classID, studentName)
	if err != nil {
		return false, "Failed to remove student."
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return false, studentName + " is not in this class."
	}
	return true, "Student removed."
}

func (c *controller) isStudentInClass(classID int, playerName string) bool {
	var exists int
	err := c.db.QueryRow(
		"SELECT 1 FROM class_students WHERE class_id = ? AND username = ?",
		classID, playerName).Scan(&exists)
	return err == nil
}

// isStudentInClassByName looks up by class name (used in access control).
func (c *controller) isStudentInClassByName(className, playerName string) bool {
	cls, err := c.getClassByName(className)
	if err != nil || cls == nil {
		return false
	}
	return c.isStudentInClass(cls.ID, playerName)
}

// ── Instance management ────────────────────────────────────────────────────

func (c *controller) createInstance(inst *instanceData) error {
	_, err := c.db.Exec(`INSERT INTO instances
		(id, class_id, created_by, institute, display_name, template_name, server_id, uuid, node_id, proxy_name, backend_addr, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		inst.ID, inst.ClassID, inst.CreatedBy, inst.Institute, inst.DisplayName, inst.TemplateName,
		inst.ServerID, inst.UUID, inst.NodeID,
		inst.ProxyName, inst.BackendAddr, inst.Status)
	return err
}

func (c *controller) updateInstanceStatus(instanceID, status string) error {
	_, err := c.db.Exec(
		"UPDATE instances SET status = ? WHERE id = ?", status, instanceID)
	return err
}

func (c *controller) getInstanceByID(id string) (*instanceData, error) {
	var inst instanceData
	err := c.db.QueryRow(`SELECT id, class_id, created_by, institute, display_name, template_name, created_at,
		server_id, uuid, node_id, proxy_name, backend_addr, status
		FROM instances WHERE id = ?`, id,
	).Scan(&inst.ID, &inst.ClassID, &inst.CreatedBy, &inst.Institute, &inst.DisplayName, &inst.TemplateName, &inst.CreatedAt,
		&inst.ServerID, &inst.UUID, &inst.NodeID,
		&inst.ProxyName, &inst.BackendAddr, &inst.Status)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &inst, nil
}

func (c *controller) getInstanceByProxyName(proxyName string) (*instanceData, error) {
	var inst instanceData
	err := c.db.QueryRow(`SELECT id, class_id, created_by, institute, display_name, template_name, created_at,
		server_id, uuid, node_id, proxy_name, backend_addr, status
		FROM instances WHERE proxy_name = ? AND status != 'deleted'`, proxyName,
	).Scan(&inst.ID, &inst.ClassID, &inst.CreatedBy, &inst.Institute, &inst.DisplayName, &inst.TemplateName, &inst.CreatedAt,
		&inst.ServerID, &inst.UUID, &inst.NodeID,
		&inst.ProxyName, &inst.BackendAddr, &inst.Status)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &inst, nil
}

func (c *controller) getInstancesForClass(classID int) ([]instanceData, error) {
	rows, err := c.db.Query(`SELECT id, class_id, created_by, institute, display_name, template_name, created_at,
		server_id, uuid, node_id, proxy_name, backend_addr, status
		FROM instances WHERE class_id = ? AND status != 'deleted'
		ORDER BY created_at DESC`, classID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInstances(rows)
}

func (c *controller) getAllActiveInstances() ([]instanceData, error) {
	return c.getFilteredActiveInstances("", "")
}

func (c *controller) getFilteredActiveInstances(institute, teacher string) ([]instanceData, error) {
	query := `SELECT id, class_id, created_by, institute, display_name, template_name, created_at,
		server_id, uuid, node_id, proxy_name, backend_addr, status
		FROM instances WHERE status != 'deleted'`
	var args []interface{}
	if institute != "" {
		query += " AND institute = ?"
		args = append(args, institute)
	}
	if teacher != "" {
		query += " AND created_by = ?"
		args = append(args, teacher)
	}
	query += " ORDER BY created_at DESC"

	rows, err := c.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInstances(rows)
}

func scanInstances(rows *sql.Rows) ([]instanceData, error) {
	var result []instanceData
	for rows.Next() {
		var inst instanceData
		if err := rows.Scan(&inst.ID, &inst.ClassID, &inst.CreatedBy, &inst.Institute, &inst.DisplayName, &inst.TemplateName,
			&inst.CreatedAt, &inst.ServerID, &inst.UUID, &inst.NodeID,
			&inst.ProxyName, &inst.BackendAddr, &inst.Status); err != nil {
			return nil, err
		}
		result = append(result, inst)
	}
	return result, rows.Err()
}

func (c *controller) deleteInstanceRecord(instanceID string) error {
	_, err := c.db.Exec(
		"UPDATE instances SET status = 'deleted' WHERE id = ?", instanceID)
	return err
}

// ── Instance invites ───────────────────────────────────────────────────────

func (c *controller) addInstanceInvite(instanceID, username string) error {
	_, err := c.db.Exec(
		"INSERT IGNORE INTO instance_invites (instance_id, username) VALUES (?, ?)",
		instanceID, username)
	return err
}

func (c *controller) removeInstanceInvite(instanceID, username string) error {
	_, err := c.db.Exec(
		"DELETE FROM instance_invites WHERE instance_id = ? AND username = ?",
		instanceID, username)
	return err
}

func (c *controller) getInstanceInvites(instanceID string) ([]string, error) {
	rows, err := c.db.Query(
		"SELECT username FROM instance_invites WHERE instance_id = ? ORDER BY username",
		instanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		result = append(result, name)
	}
	return result, rows.Err()
}

func (c *controller) isInstanceInvited(instanceID, username string) bool {
	var exists int
	err := c.db.QueryRow(
		"SELECT 1 FROM instance_invites WHERE instance_id = ? AND username = ?",
		instanceID, username).Scan(&exists)
	return err == nil
}

// ── Freeze / Watch (runtime-only, not in DB) ──────────────────────────────

func (c *controller) freezePlayer(name string) {
	c.mu.Lock()
	c.runtime.frozenPlayers[name] = true
	c.mu.Unlock()

	c.sendToPlayerServer(name, map[string]string{
		"action": "freeze",
		"player": name,
	})
}

func (c *controller) unfreezePlayer(name string) {
	c.mu.Lock()
	delete(c.runtime.frozenPlayers, name)
	delete(c.runtime.watchingPlayers, name)
	c.mu.Unlock()

	c.sendToPlayerServer(name, map[string]string{
		"action": "unfreeze",
		"player": name,
	})
}

func (c *controller) isFrozen(name string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.runtime.frozenPlayers[name]
}

func (c *controller) freezeClass(classID int) {
	for _, name := range c.getOnlineStudents(classID) {
		c.freezePlayer(name)
	}
}

func (c *controller) unfreezeClass(classID int) {
	for _, name := range c.getOnlineStudents(classID) {
		c.unfreezePlayer(name)
	}
}

func (c *controller) watchTeacher(classID int, teacherName string) {
	for _, name := range c.getOnlineStudents(classID) {
		c.mu.Lock()
		c.runtime.watchingPlayers[name] = teacherName
		c.mu.Unlock()

		c.sendToPlayerServer(name, map[string]interface{}{
			"action":  "watch",
			"player":  name,
			"teacher": teacherName,
		})
	}
}

func (c *controller) gatherClass(classID int, teacherName string) {
	tcc := proxy.Find(teacherName)
	if tcc == nil {
		return
	}
	tServer := tcc.ServerName()

	students := c.getOnlineStudents(classID)
	onlineStudents := make([]string, 0)
	needsHop := false

	for _, s := range students {
		if s == teacherName {
			continue
		}
		scc := proxy.Find(s)
		if scc == nil {
			continue
		}
		onlineStudents = append(onlineStudents, s)

		if scc.ServerName() != tServer {
			scc.Hop(tServer)
			needsHop = true
		}
	}

	if len(onlineStudents) > 0 {
		// If any students needed to hop, delay the gather command
		// to give them time to arrive on the teacher's server.
		if needsHop {
			go func() {
				time.Sleep(3 * time.Second)
				c.sendToPlayerServer(teacherName, map[string]interface{}{
					"action":  "gather",
					"players": onlineStudents,
					"target":  teacherName,
				})
			}()
		} else {
			c.sendToPlayerServer(teacherName, map[string]interface{}{
				"action":  "gather",
				"players": onlineStudents,
				"target":  teacherName,
			})
		}
	}
}

// teleportToPlayer hops the teacher to the student's server (if needed)
// and sends a tp_to command to that server.
func (c *controller) teleportToPlayer(cc *proxy.ClientConn, target string) {
	scc := proxy.Find(target)
	if scc == nil {
		cc.SendChatMsg("[Classrooms] " + target + " is not online.")
		return
	}

	teacherServer := cc.ServerName()
	studentServer := scc.ServerName()

	// Hop teacher to student's server if they're on different servers
	if teacherServer != studentServer {
		cc.Hop(studentServer)
		// Give hop time to complete before sending tp command
		go func() {
			time.Sleep(2 * time.Second)
			c.sendToPlayerServer(target, map[string]string{
				"action": "tp_to",
				"player": cc.Name(),
				"target": target,
			})
		}()
		return
	}

	c.sendToPlayerServer(target, map[string]string{
		"action": "tp_to",
		"player": cc.Name(),
		"target": target,
	})
}

// watchSingleStudent forces a single student to look at the teacher.
func (c *controller) watchSingleStudent(teacherName, studentName string) {
	c.mu.Lock()
	c.runtime.watchingPlayers[studentName] = teacherName
	c.mu.Unlock()

	c.sendToPlayerServer(studentName, map[string]interface{}{
		"action":  "watch",
		"player":  studentName,
		"teacher": teacherName,
	})
}

func (c *controller) stopWatching(classID int) {
	for _, name := range c.getOnlineStudents(classID) {
		c.mu.Lock()
		delete(c.runtime.watchingPlayers, name)
		c.mu.Unlock()

		c.sendToPlayerServer(name, map[string]string{
			"action": "unwatch",
			"player": name,
		})
	}
}

func (c *controller) isWatching(name string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.runtime.watchingPlayers[name]
}

// ── Active class/instance tracking (runtime-only) ──────────────────────────

const (
	viewOriginTeacher        = "teacher"
	viewOriginAdminClasses   = "admin_classes"
	viewOriginAdminInstances = "admin_instances"
)

func (c *controller) setActiveClass(player string, classID int) {
	c.setActiveClassWithOrigin(player, classID, viewOriginTeacher)
}

func (c *controller) setActiveClassWithOrigin(player string, classID int, origin string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.runtime.activeClass[player] = classID
	c.runtime.activeClassOrigin[player] = origin
}

func (c *controller) getActiveClass(player string) (int, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	id, ok := c.runtime.activeClass[player]
	return id, ok
}

func (c *controller) clearActiveClass(player string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.runtime.activeClass, player)
	delete(c.runtime.activeClassOrigin, player)
}

func (c *controller) setActiveInstance(player, instanceID string) {
	c.setActiveInstanceWithOrigin(player, instanceID, viewOriginTeacher)
}

func (c *controller) setActiveInstanceWithOrigin(player, instanceID, origin string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.runtime.activeInstance[player] = instanceID
	c.runtime.activeInstanceOrigin[player] = origin
}

func (c *controller) getActiveInstance(player string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	id, ok := c.runtime.activeInstance[player]
	return id, ok
}

func (c *controller) getActiveClassOrigin(player string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.runtime.activeClassOrigin[player]
}

func (c *controller) getActiveInstanceOrigin(player string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.runtime.activeInstanceOrigin[player]
}

func (c *controller) setAdminTab(player, tab string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.runtime.adminTab[player] = tab
}

func (c *controller) getAdminTab(player string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.runtime.adminTab[player]
}

func (c *controller) setAdminFilters(player, institute, teacher string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.runtime.adminInstituteFilter[player] = institute
	c.runtime.adminTeacherFilter[player] = teacher
}

func (c *controller) getAdminFilters(player string) (string, string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.runtime.adminInstituteFilter[player], c.runtime.adminTeacherFilter[player]
}

func (c *controller) clearActiveInstance(player string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.runtime.activeInstance, player)
	delete(c.runtime.activeInstanceOrigin, player)
	delete(c.runtime.adminTab, player)
	delete(c.runtime.adminInstituteFilter, player)
	delete(c.runtime.adminTeacherFilter, player)
}

// ── Helpers ────────────────────────────────────────────────────────────────

// getPublicTemplates returns template keys visible to teachers.
func (c *controller) getPublicTemplates() []string {
	var result []string
	for key, tpl := range c.cfg.Templates {
		if tpl.Public {
			result = append(result, key)
		}
	}
	sort.Strings(result)
	return result
}

// getAllTemplates returns all template keys (for admins).
func (c *controller) getAllTemplates() []string {
	var result []string
	for key := range c.cfg.Templates {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
