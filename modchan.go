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
		c.handleBridgeMessage(cc, msg)
		// Intercept internal classrooms messages
		return true
	})
}

func (c *controller) handleBridgeMessage(cc *proxy.ClientConn, msg string) {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(msg), &data); err != nil {
		return
	}
	if data["action"] != "spawnpoint_captured" {
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

		// Re-apply states after initial join
		go func() {
			time.Sleep(2 * time.Second)
			c.reapplyStates(name)
		}()
		return ""
	})

	proxy.RegisterOnLeave(func(cc *proxy.ClientConn) {
		name := cc.Name()
		c.clearActiveClass(name)
		c.clearActiveInstance(name)

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
