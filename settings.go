package main

import (
	"database/sql"
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
	StaticSpawnpoint   sql.NullString
	SpawnYaw           sql.NullFloat64
	SpawnPitch         sql.NullFloat64
}

func defaultInstanceSettings(instanceID string) instanceSettings {
	return instanceSettings{InstanceID: instanceID}
}

func (s instanceSettings) spawnString() string {
	if !s.StaticSpawnpoint.Valid {
		return ""
	}
	return s.StaticSpawnpoint.String
}

func (s instanceSettings) toLuantiConfigSettings() map[string]interface{} {
	settings := map[string]interface{}{
		"enable_damage":           s.EnableDamage,
		"enable_pvp":              s.EnablePVP,
		"mcl_enable_hunger":       s.EnableHunger,
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

func (s instanceSettings) toLuantiRuntimeSettings() map[string]interface{} {
	return map[string]interface{}{
		"enable_damage": s.EnableDamage,
		"enable_pvp":    s.EnablePVP,
	}
}

func (c *controller) getInstanceSettings(instanceID string) (*instanceSettings, error) {
	var s instanceSettings
	err := c.db.QueryRow(`SELECT instance_id, enable_damage, enable_pvp, mcl_enable_hunger,
		mobs_spawn, only_peaceful_mobs, mcl_explosions_griefing, static_spawnpoint,
		spawn_yaw, spawn_pitch
		FROM instance_settings WHERE instance_id = ?`, instanceID).Scan(
		&s.InstanceID, &s.EnableDamage, &s.EnablePVP, &s.EnableHunger,
		&s.MobsSpawn, &s.OnlyPeacefulMobs, &s.ExplosionsGriefing, &s.StaticSpawnpoint,
		&s.SpawnYaw, &s.SpawnPitch)
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
		 only_peaceful_mobs, mcl_explosions_griefing, static_spawnpoint, spawn_yaw, spawn_pitch)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			enable_damage = VALUES(enable_damage),
			enable_pvp = VALUES(enable_pvp),
			mcl_enable_hunger = VALUES(mcl_enable_hunger),
			mobs_spawn = VALUES(mobs_spawn),
			only_peaceful_mobs = VALUES(only_peaceful_mobs),
			mcl_explosions_griefing = VALUES(mcl_explosions_griefing),
			static_spawnpoint = VALUES(static_spawnpoint),
			spawn_yaw = VALUES(spawn_yaw),
			spawn_pitch = VALUES(spawn_pitch)`,
		s.InstanceID, s.EnableDamage, s.EnablePVP, s.EnableHunger, s.MobsSpawn,
		s.OnlyPeacefulMobs, s.ExplosionsGriefing, nullableString(s.StaticSpawnpoint),
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
			c.sendToPlayerServer(cc.Name(), msg)
			return true
		}
	}
	return false
}
