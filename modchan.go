package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	proxy "github.com/HimbeerserverDE/mt-multiserver-proxy"
)

func (c *controller) registerModChannel() {
	proxy.RegisterOnSrvModChanMsg(func(cc *proxy.ClientConn, channel, sender, msg string) bool {
		if channel != modChannel {
			return false
		}
		c.handleBridgeMessage(cc, sender, msg)
		// Intercept internal classrooms messages
		return true
	})
}

func (c *controller) handleBridgeMessage(cc *proxy.ClientConn, sender, msg string) {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(msg), &data); err != nil {
		return
	}
	action, _ := data["action"].(string)
	if action == "open_portal_worlds" {
		player, _ := data["player"].(string)
		if sender != "" || player != cc.Name() || cc.ServerName() != c.cfg.LobbyServer {
			return
		}
		c.showPortalWorlds(cc)
		return
	}
	if action == "return_hub" {
		player, _ := data["player"].(string)
		if sender != "" || player != cc.Name() || !c.isCurrentServerClassInstance(cc) {
			return
		}
		if err := c.hopPlayer(cc, c.cfg.LobbyServer); err != nil {
			cc.SendChatMsg("[Classrooms] Failed to return to the HUB: " + err.Error())
			return
		}
		c.clearPortalVisitor(player)
		return
	}
	if action == "open_classes" {
		player, _ := data["player"].(string)
		if player != cc.Name() || !c.hasClassPanelAccess(player) {
			return
		}
		c.showMainDashboard(cc)
		return
	}
	if action != "spawnpoint_captured" {
		return
	}
	requestID, _ := data["request_id"].(string)
	if requestID == "" {
		return
	}
	c.mu.Lock()
	capture, ok := c.runtime.spawnCaptures[requestID]
	if ok {
		delete(c.runtime.spawnCaptures, requestID)
	}
	c.mu.Unlock()
	if !ok {
		return
	}

	posString, _ := data["pos_string"].(string)
	if strings.TrimSpace(posString) == "" {
		pos, _ := data["pos"].(map[string]interface{})
		x, okX := numberField(pos, "x")
		y, okY := numberField(pos, "y")
		z, okZ := numberField(pos, "z")
		if !okX || !okY || !okZ {
			return
		}
		posString = fmt.Sprintf("(%.2f,%.2f,%.2f)", x, y, z)
	}

	settings, err := c.getInstanceSettingsOrDefault(capture.InstanceID)
	if err != nil {
		return
	}
	settings.StaticSpawnpoint = sql.NullString{
		String: posString,
		Valid:  true,
	}
	if yaw, ok := numberField(data, "yaw"); ok {
		settings.SpawnYaw = sql.NullFloat64{Float64: yaw, Valid: true}
	}
	if pitch, ok := numberField(data, "pitch"); ok {
		settings.SpawnPitch = sql.NullFloat64{Float64: pitch, Valid: true}
	}
	if err := c.saveInstanceSettings(settings); err != nil {
		log.Printf("[%s] failed to save captured spawnpoint for %s: %v", pluginName, capture.InstanceID, err)
		return
	}
	log.Printf("[%s] saved captured spawnpoint for %s: %s", pluginName, capture.InstanceID, posString)
	teacher := proxy.Find(capture.Teacher)
	if teacher == nil {
		teacher = cc
	}
	if teacher != nil {
		teacher.SendChatMsg("[Classrooms] Spawnpoint saved. Restart the instance to apply it as startup spawn.")
		c.showInstanceSettings(teacher, capture.InstanceID)
	}
}

func numberField(m map[string]interface{}, key string) (float64, bool) {
	if m == nil {
		return 0, false
	}
	v, ok := m[key].(float64)
	return v, ok
}

func (c *controller) registerJoinLeave() {
	proxy.RegisterOnJoin(func(cc *proxy.ClientConn) string {
		name := cc.Name()
		go c.ensureChannelJoin(cc)
		c.scheduleReapplyStates(name, 2*time.Second)
		return ""
	})

	proxy.RegisterOnLeave(func(cc *proxy.ClientConn) {
		name := cc.Name()
		c.clearActiveClass(name)
		c.clearActiveInstance(name)
		c.clearPortalVisitor(name)

		c.mu.Lock()
		delete(c.runtime.watchingPlayers, name)
		// If a teacher leaves, stop watching for all their students
		for student, teacher := range c.runtime.watchingPlayers {
			if teacher == name {
				delete(c.runtime.watchingPlayers, student)
			}
		}
		c.mu.Unlock()
	})
}

func (c *controller) reapplyStates(playerName string) {
	c.reapplyTeacherContext(playerName)
	c.reapplyPortalVisitorContext(playerName)

	if c.isFrozen(playerName) {
		c.sendToPlayerServer(playerName, map[string]string{
			"action": "freeze",
			"player": playerName,
		})
	}
	if teacher := c.isWatching(playerName); teacher != "" {
		c.sendToPlayerServer(playerName, map[string]interface{}{
			"action":  "watch",
			"player":  playerName,
			"teacher": teacher,
		})
	}
}

func (c *controller) reapplyTeacherContext(playerName string) {
	cc := proxy.Find(playerName)
	if cc == nil || cc.ServerName() == "" {
		return
	}

	if !c.hasClassPanelAccess(playerName) {
		// First restore survival and remove managed privileges, then remove the
		// permanent panel item. This also handles a live Assistance removal.
		c.sendToPlayerServer(playerName, map[string]string{
			"action": "clear_teacher_defaults",
			"player": playerName,
		})
		c.sendToPlayerServer(playerName, map[string]string{
			"action": "clear_teacher_access",
			"player": playerName,
		})
		return
	}

	// Proxy administrators already manage their own backend privileges. Give
	// them the teacher panel without changing gamemode, fly, fast, or any
	// other server-local privilege.
	if c.isAdmin(playerName) {
		if !c.sendToPlayerServer(playerName, map[string]string{
			"action": "set_teacher_access",
			"player": playerName,
		}) {
			log.Printf("[%s] failed to apply admin teacher access for %s on %s", pluginName, playerName, cc.ServerName())
		}
		return
	}

	inst, err := c.getInstanceByProxyName(cc.ServerName())
	if err != nil {
		log.Printf("[%s] failed to resolve teacher context for %s on %s: %v", pluginName, playerName, cc.ServerName(), err)
		return
	}

	action := "clear_teacher_defaults"
	blockExchangeAccess := false
	if inst != nil && inst.ClassID != nil && c.canEditClassStudents(*inst.ClassID, playerName) {
		action = "set_teacher_defaults"
		blockExchangeAccess = c.canManageClass(*inst.ClassID, playerName)
	}
	if !c.sendToPlayerServer(playerName, map[string]interface{}{
		"action":               action,
		"player":               playerName,
		"blockexchange_access": blockExchangeAccess,
	}) {
		log.Printf("[%s] failed to apply teacher context %s for %s on %s", pluginName, action, playerName, cc.ServerName())
	}
}

func (c *controller) scheduleReapplyStates(playerName string, delay time.Duration) {
	go func() {
		time.Sleep(delay)
		c.reapplyStates(playerName)
	}()
}

func (c *controller) hopPlayer(cc *proxy.ClientConn, serverName string) error {
	if err := cc.Hop(serverName); err != nil {
		return err
	}
	c.scheduleReapplyStates(cc.Name(), 3*time.Second)
	return nil
}

func (c *controller) ensureChannelJoin(cc *proxy.ClientConn) {
	for attempt := 0; attempt < 12; attempt++ {
		if cc.ServerName() == "" {
			time.Sleep(1 * time.Second)
			continue
		}
		if cc.IsModChanJoined(modChannel) {
			return
		}

		joined := cc.JoinModChan(modChannel)
		select {
		case ok := <-joined:
			if ok {
				return
			}
		case <-time.After(3 * time.Second):
		}
		time.Sleep(1 * time.Second)
	}
	log.Printf("[%s] failed to join modchannel %q for %s", pluginName, modChannel, cc.Name())
}
