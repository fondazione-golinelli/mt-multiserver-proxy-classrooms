package main

import (
	"fmt"
	"strings"

	proxy "github.com/HimbeerserverDE/mt-multiserver-proxy"
)

func worldTimePresetValue(preset string) (float64, bool) {
	switch preset {
	case "day":
		return 0.25, false
	case "evening":
		return 0.6, false
	case "night":
		return 0.0, false
	case "stop":
		return 0.0, true
	default:
		return 0.25, false
	}
}

func worldWeatherPresetValue(preset string) string {
	switch preset {
	case "sunny":
		return "none"
	case "rain":
		return "rain"
	default:
		return "none"
	}
}

func (c *controller) showWorldControls(cc *proxy.ClientConn, instanceID string) {
	if cc == nil {
		return
	}
	if instanceID != "" {
		c.setActiveInstanceWithOrigin(cc.Name(), instanceID, c.getActiveInstanceOrigin(cc.Name()))
	}
	stopped := c.isInstanceTimeStopped(instanceID)

	var b strings.Builder
	b.WriteString("formspec_version[6]")
	b.WriteString("size[10,8.4]")
	b.WriteString(fmt.Sprintf("bgcolor[%s;true]", headerColor))

	b.WriteString(box(0, 0, 10, 0.95, panel))
	b.WriteString(btn(0.2, 0.18, 1.25, 0.52, "btn_back", "Back"))
	b.WriteString(coloredLbl(1.75, 0.42, light, "World Controls"))
	b.WriteString(box(0, 0.95, 10, 0.04, accent))

	b.WriteString(box(0.4, 1.25, 9.2, 2.4, panel))
	b.WriteString(coloredLbl(0.65, 1.6, muted, "Time"))
	b.WriteString(btn(0.7, 1.95, 2.2, 0.7, "btn_world_day", "Day"))
	b.WriteString(btn(3.2, 1.95, 2.2, 0.7, "btn_world_evening", "Evening"))
	b.WriteString(btn(5.7, 1.95, 2.2, 0.7, "btn_world_night", "Night"))
	if stopped {
		b.WriteString(btn(8.2, 1.95, 1.45, 0.7, "btn_world_resume_time", "Resume"))
	} else {
		b.WriteString(btn(8.2, 1.95, 1.45, 0.7, "btn_world_stop_time", "Stop"))
	}

	b.WriteString(box(0.4, 3.95, 9.2, 2.4, panel))
	b.WriteString(coloredLbl(0.65, 4.3, muted, "Weather"))
	b.WriteString(btn(0.7, 4.65, 2.2, 0.7, "btn_world_sunny", "Sunny"))
	b.WriteString(btn(3.2, 4.65, 2.2, 0.7, "btn_world_rain", "Rain"))

	b.WriteString(box(0.4, 6.7, 9.2, 0.95, panel))
	if stopped {
		b.WriteString(coloredLbl(0.65, 7.08, muted, "Time is currently stopped. Use Resume to restart time flow."))
	} else {
		b.WriteString(coloredLbl(0.65, 7.08, muted, "The world controls apply to the current instance server."))
	}

	cc.ShowFormspec("classrooms:world_controls", b.String())
}

func (c *controller) applyWorldControls(cc *proxy.ClientConn, preset, weather string, stopTime bool) {
	if cc == nil {
		return
	}
	player := cc.Name()
	instID, _ := c.getActiveInstance(player)
	if preset != "" {
		timeValue, stop := worldTimePresetValue(preset)
		if stopTime {
			stop = true
			// Preserve the current in-world time while freezing it.
			timeValue = -1
		}
		c.sendToPlayerServer(player, map[string]interface{}{
			"action":    "set_world_time",
			"player":    player,
			"preset":    preset,
			"time":      timeValue,
			"stop_time": stop,
		})
		c.setInstanceTimeStopped(instID, stop)
	} else if stopTime {
		c.sendToPlayerServer(player, map[string]interface{}{
			"action":    "set_world_time",
			"player":    player,
			"preset":    "",
			"time":      -1,
			"stop_time": true,
		})
		c.setInstanceTimeStopped(instID, true)
	}
	if weather != "" {
		c.sendToPlayerServer(player, map[string]interface{}{
			"action":  "set_world_weather",
			"player":  player,
			"preset":  preset,
			"weather": worldWeatherPresetValue(weather),
		})
	}
	if !stopTime && preset == "" {
		c.sendToPlayerServer(player, map[string]interface{}{
			"action":    "set_world_time",
			"player":    player,
			"preset":    "",
			"time":      -1,
			"stop_time": false,
		})
		c.setInstanceTimeStopped(instID, false)
	}
}
