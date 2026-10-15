package main

import (
	"log"
	"time"
	_ "time/tzdata" // the proxy image may lack a zoneinfo database

	proxy "github.com/HimbeerserverDE/mt-multiserver-proxy"
)

// ── Teacher visits to worlds ────────────────────────────────────────────────
//
// The proxy has no hook for server changes, so a poll compares every
// player's server with the previous poll. Visits shorter than the interval
// can be missed; that is fine for "when did the teacher last go there".

const (
	visitPollInterval = 10 * time.Second
	visitsKept        = 20 // per world
)

type worldVisit struct {
	Username  string
	VisitedAt time.Time
}

func (c *controller) startVisitTracker() {
	go func() {
		ticker := time.NewTicker(visitPollInterval)
		defer ticker.Stop()
		lastServer := make(map[string]string)
		for range ticker.C {
			lastServer = c.trackVisits(lastServer)
		}
	}()
}

func (c *controller) trackVisits(lastServer map[string]string) map[string]string {
	current := make(map[string]string)
	for cc := range proxy.Clts() {
		name, srv := cc.Name(), cc.ServerName()
		if name == "" || srv == "" {
			continue
		}
		current[name] = srv
		if lastServer[name] == srv {
			continue
		}
		inst, err := c.getInstanceByProxyName(srv)
		if err != nil || inst == nil || !c.isWorldTeacher(inst, name) {
			continue
		}
		if err := c.recordVisit(inst.ID, name); err != nil {
			log.Printf("[%s] failed to record visit of %s to %s: %v", pluginName, name, inst.ID, err)
		}
	}
	return current
}

// isWorldTeacher: the creator of the world or a teacher of its class.
func (c *controller) isWorldTeacher(inst *instanceData, name string) bool {
	if inst.CreatedBy == name {
		return true
	}
	if inst.ClassID == nil {
		return false
	}
	cls, err := c.getClassByID(*inst.ClassID)
	return err == nil && cls != nil && (cls.CreatedBy == name || c.isLinkedTeacherInClass(cls.ID, name))
}

func (c *controller) recordVisit(instanceID, name string) error {
	if _, err := c.db.Exec("INSERT INTO instance_visits (instance_id, username) VALUES (?, ?)", instanceID, name); err != nil {
		return err
	}
	_, err := c.db.Exec(`DELETE FROM instance_visits WHERE instance_id = ? AND id NOT IN (
		SELECT id FROM (SELECT id FROM instance_visits WHERE instance_id = ? ORDER BY id DESC LIMIT ?) AS kept)`,
		instanceID, instanceID, visitsKept)
	return err
}

func (c *controller) getWorldVisits(instanceID string, limit int) []worldVisit {
	rows, err := c.db.Query(`SELECT username, visited_at FROM instance_visits
		WHERE instance_id = ? ORDER BY id DESC LIMIT ?`, instanceID, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []worldVisit
	for rows.Next() {
		var v worldVisit
		if rows.Scan(&v.Username, &v.VisitedAt) == nil {
			result = append(result, v)
		}
	}
	return result
}

// getLastWorldVisits maps every world with visits to its latest one.
func (c *controller) getLastWorldVisits() map[string]worldVisit {
	result := make(map[string]worldVisit)
	rows, err := c.db.Query(`SELECT v.instance_id, v.username, v.visited_at FROM instance_visits v
		JOIN (SELECT MAX(id) AS id FROM instance_visits GROUP BY instance_id) latest ON latest.id = v.id`)
	if err != nil {
		return result
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var v worldVisit
		if rows.Scan(&id, &v.Username, &v.VisitedAt) == nil {
			result[id] = v
		}
	}
	return result
}

var displayZone = func() *time.Location {
	if loc, err := time.LoadLocation("Europe/Rome"); err == nil {
		return loc
	}
	return time.UTC
}()

// fmtTime formats a database time (UTC) in the schools' time zone.
func fmtTime(t time.Time) string {
	return t.In(displayZone).Format("02/01/2006 15:04")
}
