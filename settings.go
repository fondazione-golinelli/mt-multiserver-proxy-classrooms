package main

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	proxy "github.com/HimbeerserverDE/mt-multiserver-proxy"
)

type instanceSettings struct {
	InstanceID         string
	EnableDamage       bool
	EnablePVP          bool
	EnableHunger       bool
	MobsSpawn          bool
	OnlyPeacefulMobs   bool
	ExplosionsGriefing bool
	StudentFly         bool // students may fly (teachers always can)
	StudentCreative    bool // students play in creative mode
	WorldLocked        bool // students can look but not build or interact
	StaticSpawnpoint   sql.NullString
	SpawnYaw           sql.NullFloat64
	SpawnPitch         sql.NullFloat64
}

// defaultInstanceSettings is the classroom-safe baseline for new teacher
// instances: no damage, PvP, or hunger, and only peaceful mobs spawn.
func defaultInstanceSettings(instanceID string) instanceSettings {
	return instanceSettings{
		InstanceID:       instanceID,
		MobsSpawn:        true,
		OnlyPeacefulMobs: true,
	}
}

// toPelicanEnvironment maps settings to the Luanti egg's CLASSROOMS_* startup
// variables, so a freshly provisioned instance boots with them before the
// bridge has written any pending settings.
//
// Damage, PvP and hunger always start on: Mineclonia only builds its health
// and hunger machinery when the server starts with them, and crashes when
// enable_damage changes while it runs. The teacher's choices are live world
// rules applied by the bridge (set_world_rules).
func (s instanceSettings) toPelicanEnvironment() map[string]string {
	return map[string]string{
		"CLASSROOMS_ENABLE_DAMAGE":       "true",
		"CLASSROOMS_ENABLE_PVP":          "true",
		"CLASSROOMS_ENABLE_HUNGER":       "true",
		"CLASSROOMS_MOBS_SPAWN":          fmt.Sprintf("%t", s.MobsSpawn),
		"CLASSROOMS_ONLY_PEACEFUL_MOBS":  fmt.Sprintf("%t", s.OnlyPeacefulMobs),
		"CLASSROOMS_EXPLOSIONS_GRIEFING": fmt.Sprintf("%t", s.ExplosionsGriefing),
	}
}

func (s instanceSettings) spawnString() string {
	if !s.StaticSpawnpoint.Valid {
		return ""
	}
	return s.StaticSpawnpoint.String
}

func (s instanceSettings) toLuantiConfigSettings() map[string]interface{} {
	settings := map[string]interface{}{
		// Engine support stays on; see toPelicanEnvironment.
		"enable_damage":           true,
		"enable_pvp":              true,
		"mcl_enable_hunger":       true,
		"mobs_spawn":              s.MobsSpawn,
		"only_peaceful_mobs":      s.OnlyPeacefulMobs,
		"mcl_explosions_griefing": s.ExplosionsGriefing,
	}
	if s.StaticSpawnpoint.Valid && strings.TrimSpace(s.StaticSpawnpoint.String) != "" {
		settings["static_spawnpoint"] = s.StaticSpawnpoint.String
	}
	if s.SpawnYaw.Valid {
		settings["classrooms_spawn_yaw"] = s.SpawnYaw.Float64
	}
	if s.SpawnPitch.Valid {
		settings["classrooms_spawn_pitch"] = s.SpawnPitch.Float64
	}
	return settings
}

// toLuantiRuntimeSettings is empty: engine settings are never changed while a
// world runs (enable_damage crashes Mineclonia); rules go in set_world_rules.
func (s instanceSettings) toLuantiRuntimeSettings() map[string]interface{} {
	return map[string]interface{}{}
}

// worldRulesMessage carries the live rules the bridge applies: hurt, PvP and
// hunger (teachers, creative and flying players are always exempt) and the
// world lock (students look but don't touch).
func worldRulesMessage(s instanceSettings) map[string]interface{} {
	return map[string]interface{}{
		"action": "set_world_rules",
		"damage": s.EnableDamage,
		"pvp":    s.EnablePVP,
		"hunger": s.EnableHunger,
		"locked": s.WorldLocked,
	}
}

func (c *controller) getInstanceSettings(instanceID string) (*instanceSettings, error) {
	var s instanceSettings
	err := c.db.QueryRow(`SELECT instance_id, enable_damage, enable_pvp, mcl_enable_hunger,
		mobs_spawn, only_peaceful_mobs, mcl_explosions_griefing, student_fly, student_creative, world_locked,
		static_spawnpoint, spawn_yaw, spawn_pitch
		FROM instance_settings WHERE instance_id = ?`, instanceID).Scan(
		&s.InstanceID, &s.EnableDamage, &s.EnablePVP, &s.EnableHunger,
		&s.MobsSpawn, &s.OnlyPeacefulMobs, &s.ExplosionsGriefing, &s.StudentFly, &s.StudentCreative, &s.WorldLocked,
		&s.StaticSpawnpoint, &s.SpawnYaw, &s.SpawnPitch)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (c *controller) getInstanceSettingsOrDefault(instanceID string) (instanceSettings, error) {
	s, err := c.getInstanceSettings(instanceID)
	if err != nil {
		return instanceSettings{}, err
	}
	if s == nil {
		return defaultInstanceSettings(instanceID), nil
	}
	return *s, nil
}

func (c *controller) saveInstanceSettings(s instanceSettings) error {
	_, err := c.db.Exec(`INSERT INTO instance_settings
		(instance_id, enable_damage, enable_pvp, mcl_enable_hunger, mobs_spawn,
		 only_peaceful_mobs, mcl_explosions_griefing, student_fly, student_creative, world_locked,
		 static_spawnpoint, spawn_yaw, spawn_pitch)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			enable_damage = VALUES(enable_damage),
			enable_pvp = VALUES(enable_pvp),
			mcl_enable_hunger = VALUES(mcl_enable_hunger),
			mobs_spawn = VALUES(mobs_spawn),
			only_peaceful_mobs = VALUES(only_peaceful_mobs),
			mcl_explosions_griefing = VALUES(mcl_explosions_griefing),
			student_fly = VALUES(student_fly),
			student_creative = VALUES(student_creative),
			world_locked = VALUES(world_locked),
			static_spawnpoint = VALUES(static_spawnpoint),
			spawn_yaw = VALUES(spawn_yaw),
			spawn_pitch = VALUES(spawn_pitch)`,
		s.InstanceID, s.EnableDamage, s.EnablePVP, s.EnableHunger, s.MobsSpawn,
		s.OnlyPeacefulMobs, s.ExplosionsGriefing, s.StudentFly, s.StudentCreative, s.WorldLocked, nullableString(s.StaticSpawnpoint),
		nullableFloat(s.SpawnYaw), nullableFloat(s.SpawnPitch))
	return err
}

func nullableString(v sql.NullString) any {
	if !v.Valid {
		return nil
	}
	return v.String
}

func nullableFloat(v sql.NullFloat64) any {
	if !v.Valid {
		return nil
	}
	return v.Float64
}

func boolField(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "true" || value == "1" || value == "yes" || value == "on"
}

// studentAbilitiesMessage tells the bridge what students may do: fly and
// creative mode. Without "player" it applies to everyone on the server.
func studentAbilitiesMessage(s instanceSettings, player string) map[string]interface{} {
	msg := map[string]interface{}{
		"action":   "set_student_abilities",
		"fly":      s.StudentFly,
		"creative": s.StudentCreative,
	}
	if player != "" {
		msg["player"] = player
	}
	return msg
}

// reapplyStudentAbilities sends a world's student abilities to a player who
// just joined it; the bridge ignores it for staff and spectators.
func (c *controller) reapplyStudentAbilities(playerName string) {
	cc := proxy.Find(playerName)
	if cc == nil || cc.ServerName() == "" || cc.ServerName() == c.cfg.LobbyServer {
		return
	}
	inst, err := c.getInstanceByProxyName(cc.ServerName())
	if err != nil || inst == nil {
		return
	}
	settings, err := c.getInstanceSettingsOrDefault(inst.ID)
	if err != nil {
		return
	}
	c.sendToPlayerServer(playerName, worldRulesMessage(settings))
	c.sendToPlayerServer(playerName, studentAbilitiesMessage(settings, playerName))
}

func (c *controller) sendSettingsToInstance(inst *instanceData, settings instanceSettings) bool {
	if inst == nil {
		return false
	}
	msg := map[string]interface{}{
		"action":           "set_settings",
		"config_settings":  settings.toLuantiConfigSettings(),
		"runtime_settings": settings.toLuantiRuntimeSettings(),
	}
	for cc := range proxy.Clts() {
		if cc.ServerName() == inst.ProxyName {
			if c.sendToPlayerServer(cc.Name(), msg) {
				c.sendToPlayerServer(cc.Name(), worldRulesMessage(settings))
				c.sendToPlayerServer(cc.Name(), studentAbilitiesMessage(settings, ""))
				log.Printf("[%s] sent saved settings for instance %s through %s", pluginName, inst.ID, cc.Name())
				return true
			}
			log.Printf("[%s] failed to send saved settings for instance %s through %s", pluginName, inst.ID, cc.Name())
		}
	}
	return false
}
