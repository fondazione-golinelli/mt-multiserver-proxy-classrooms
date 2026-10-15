package main

import (
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	proxy "github.com/HimbeerserverDE/mt-multiserver-proxy"
)

// ── Institutes ──────────────────────────────────────────────────────────────
//
// Teachers belong to at most one institute. A class counts towards the
// institute of its owner. Limits of 0 mean "no limit"; administrators are
// never limited.

type instituteData struct {
	ID                int
	Name              string
	MaxClasses        int
	MaxWorldsPerClass int
	Teachers          int
	Classes           int
	CreatedAt         time.Time
}

func (c *controller) listInstitutes() ([]instituteData, error) {
	rows, err := c.db.Query(`SELECT i.id, i.name, i.max_classes, i.max_worlds_per_class, i.created_at,
			(SELECT COUNT(*) FROM teachers t WHERE t.institute_id = i.id),
			(SELECT COUNT(*) FROM classes cl JOIN teachers t ON t.username = cl.created_by WHERE t.institute_id = i.id)
		FROM institutes i ORDER BY i.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []instituteData
	for rows.Next() {
		var in instituteData
		if err := rows.Scan(&in.ID, &in.Name, &in.MaxClasses, &in.MaxWorldsPerClass, &in.CreatedAt,
			&in.Teachers, &in.Classes); err != nil {
			return nil, err
		}
		result = append(result, in)
	}
	return result, rows.Err()
}

func (c *controller) getInstitute(id int) (*instituteData, error) {
	var in instituteData
	err := c.db.QueryRow("SELECT id, name, max_classes, max_worlds_per_class, created_at FROM institutes WHERE id = ?", id).
		Scan(&in.ID, &in.Name, &in.MaxClasses, &in.MaxWorldsPerClass, &in.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &in, nil
}

// parseInstituteInput validates the fields of the Institutes tab.
func parseInstituteInput(name, maxClasses, maxWorlds string) (string, int, int, string) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 100 {
		return "", 0, 0, T("Institute name must be 1-100 characters.")
	}
	limits := [2]int{}
	for i, v := range []string{maxClasses, maxWorlds} {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 10000 {
			return "", 0, 0, T("Limits must be whole numbers; 0 means no limit.")
		}
		limits[i] = n
	}
	return name, limits[0], limits[1], ""
}

func (c *controller) createInstitute(name, maxClasses, maxWorlds string) (bool, string) {
	name, mc, mw, msg := parseInstituteInput(name, maxClasses, maxWorlds)
	if msg != "" {
		return false, msg
	}
	if _, err := c.db.Exec("INSERT INTO institutes (name, max_classes, max_worlds_per_class) VALUES (?, ?, ?)",
		name, mc, mw); err != nil {
		return false, T("An institute with that name already exists.")
	}
	return true, T("Institute @1 created.", name)
}

func (c *controller) updateInstitute(id int, name, maxClasses, maxWorlds string) (bool, string) {
	name, mc, mw, msg := parseInstituteInput(name, maxClasses, maxWorlds)
	if msg != "" {
		return false, msg
	}
	old, err := c.getInstitute(id)
	if err != nil || old == nil {
		return false, T("Institute not found.")
	}
	if _, err := c.db.Exec("UPDATE institutes SET name = ?, max_classes = ?, max_worlds_per_class = ? WHERE id = ?",
		name, mc, mw, id); err != nil {
		return false, T("An institute with that name already exists.")
	}
	if old.Name != name {
		// Worlds keep the institute name they were created with: follow the rename.
		if _, err := c.db.Exec("UPDATE instances SET institute = ? WHERE institute = ?", name, old.Name); err != nil {
			log.Printf("[%s] failed to rename institute on worlds: %v", pluginName, err)
		}
	}
	return true, T("Institute @1 saved.", name)
}

// deleteInstitute deletes the institute, the classes of its teachers with
// their worlds, and takes the teacher role from its teachers.
func (c *controller) deleteInstitute(id int) (bool, string) {
	in, err := c.getInstitute(id)
	if err != nil || in == nil {
		return false, T("Institute not found.")
	}
	teachers, err := c.queryStrings("SELECT username FROM teachers WHERE institute_id = ?", id)
	if err != nil {
		return false, T("Database error.")
	}
	classIDs, err := c.queryInts(`SELECT cl.id FROM classes cl JOIN teachers t ON t.username = cl.created_by
		WHERE t.institute_id = ?`, id)
	if err != nil {
		return false, T("Database error.")
	}
	var worlds []instanceData
	for _, classID := range classIDs {
		w, err := c.getInstancesForClass(classID)
		if err != nil {
			return false, T("Database error.")
		}
		worlds = append(worlds, w...)
	}

	tx, err := c.db.Begin()
	if err != nil {
		return false, T("Database error.")
	}
	defer tx.Rollback()
	for _, classID := range classIDs {
		if _, err := tx.Exec("DELETE FROM classes WHERE id = ?", classID); err != nil {
			return false, T("Failed to delete class.")
		}
	}
	if _, err := tx.Exec("DELETE FROM teachers WHERE institute_id = ?", id); err != nil {
		return false, T("Database error.")
	}
	if _, err := tx.Exec("DELETE FROM institutes WHERE id = ?", id); err != nil {
		return false, T("Database error.")
	}
	if err := tx.Commit(); err != nil {
		return false, T("Database error.")
	}

	log.Printf("[%s] deleted institute %s: %d classes, %d worlds, %d teachers",
		pluginName, in.Name, len(classIDs), len(worlds), len(teachers))
	c.deleteInstancesAsync(worlds)
	for _, t := range teachers {
		if proxy.Find(t) != nil {
			c.scheduleReapplyStates(t, 0)
		}
	}
	return true, T("Institute @1 deleted: @2 classes, @3 worlds, @4 teachers are now normal users.",
		in.Name, len(classIDs), len(worlds), len(teachers))
}

// teacherInstituteLimits returns the institute of a teacher, or nil.
func (c *controller) teacherInstituteLimits(teacher string) *instituteData {
	var id int
	err := c.db.QueryRow("SELECT COALESCE(institute_id, 0) FROM teachers WHERE username = ?", teacher).Scan(&id)
	if err != nil || id == 0 {
		return nil
	}
	in, _ := c.getInstitute(id)
	return in
}

// instituteClassUsage returns the teacher's institute and how many of its
// classes are used; max 0 means no limit. ok is false without an institute.
func (c *controller) instituteClassUsage(teacher string) (in *instituteData, used int, ok bool) {
	in = c.teacherInstituteLimits(teacher)
	if in == nil {
		return nil, 0, false
	}
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM classes cl JOIN teachers t ON t.username = cl.created_by
		WHERE t.institute_id = ?`, in.ID).Scan(&used); err != nil {
		return nil, 0, false
	}
	return in, used, true
}

// classWorldLimit returns the world limit of a class (0 = none), set by the
// institute of the class owner.
func (c *controller) classWorldLimit(cls *classData) int {
	if in := c.teacherInstituteLimits(cls.CreatedBy); in != nil {
		return in.MaxWorldsPerClass
	}
	return 0
}

// classLimitMessage explains why teacher cannot create another class, or "".
func (c *controller) classLimitMessage(teacher string) string {
	in := c.teacherInstituteLimits(teacher)
	if in == nil || in.MaxClasses == 0 {
		return ""
	}
	var n int
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM classes cl JOIN teachers t ON t.username = cl.created_by
		WHERE t.institute_id = ?`, in.ID).Scan(&n); err != nil {
		return T("Database error.")
	}
	if n >= in.MaxClasses {
		return T("@1 has reached its limit of @2 classes. Ask an administrator.", in.Name, in.MaxClasses)
	}
	return ""
}

// worldLimitMessage explains why a class cannot get another world, or "".
func (c *controller) worldLimitMessage(classID int) string {
	cls, err := c.getClassByID(classID)
	if err != nil || cls == nil {
		return ""
	}
	in := c.teacherInstituteLimits(cls.CreatedBy)
	if in == nil || in.MaxWorldsPerClass == 0 {
		return ""
	}
	var n int
	if err := c.db.QueryRow("SELECT COUNT(*) FROM instances WHERE class_id = ? AND status != 'deleted'", classID).Scan(&n); err != nil {
		return T("Database error.")
	}
	if n >= in.MaxWorldsPerClass {
		return T("This class has reached the limit of @1 worlds set by @2. Delete a world first.", in.MaxWorldsPerClass, in.Name)
	}
	return ""
}

// deleteInstancesAsync deletes worlds whose class is gone. Each deletion
// stops the server and removes it from Pelican, which takes a while.
func (c *controller) deleteInstancesAsync(worlds []instanceData) {
	if len(worlds) == 0 {
		return
	}
	go func() {
		for i := range worlds {
			if err := c.deleteInstance(&worlds[i]); err != nil {
				log.Printf("[%s] failed to delete world %s: %v", pluginName, worlds[i].ID, err)
			}
		}
	}()
}

func (c *controller) queryStrings(query string, args ...any) ([]string, error) {
	rows, err := c.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

func (c *controller) queryInts(query string, args ...any) ([]int, error) {
	rows, err := c.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []int
	for rows.Next() {
		var n int
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		result = append(result, n)
	}
	return result, rows.Err()
}

// instituteDropdown is a dropdown of "No institute" and every institute.
// Submitted with index_event, so instituteFromIndex maps the value back.
func instituteDropdown(x, y, w, h float64, name string, institutes []instituteData, selectedID int) string {
	items := []string{fmtEsc(T("No institute"))}
	selected := 1
	for i, in := range institutes {
		items = append(items, fmtEsc(in.Name))
		if in.ID == selectedID {
			selected = i + 2
		}
	}
	return fmt.Sprintf("dropdown[%g,%g;%g,%g;%s;%s;%d;true]", x, y, w, h, fmtEsc(name), strings.Join(items, ","), selected)
}

// instituteFromIndex maps a dropdown index to an institute ID (0 = none).
func instituteFromIndex(value string, institutes []instituteData) (int, bool) {
	idx, err := strconv.Atoi(value)
	if err != nil || idx < 1 || idx > len(institutes)+1 {
		return 0, false
	}
	if idx == 1 {
		return 0, true
	}
	return institutes[idx-2].ID, true
}
