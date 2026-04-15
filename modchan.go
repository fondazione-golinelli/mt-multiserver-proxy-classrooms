package main

import (
	"log"
	"time"

	proxy "github.com/HimbeerserverDE/mt-multiserver-proxy"
)

func (c *controller) registerModChannel() {
	proxy.RegisterOnSrvModChanMsg(func(cc *proxy.ClientConn, channel, sender, msg string) bool {
		if channel != modChannel {
			return false
		}
		// Intercept internal classrooms messages
		return true
	})
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
