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

func shouldResumeWorldTime(preset, weather string, stopTime bool) bool {
	return !stopTime && preset == "" && weather == ""
}

func (c *controller) showWorldControls(cc *proxy.ClientConn, instanceID string) {
	if cc == nil {
		return
	}
	if instanceID != "" {
		c.setActiveInstanceWithOrigin(cc.Name(), instanceID, c.getActiveInstanceOrigin(cc.Name()))
	} else {
		instanceID, _ = c.getActiveInstance(cc.Name())
	}
	inst, err := c.getInstanceByID(instanceID)
	if err != nil || inst == nil {
		c.showMainDashboard(cc)
		return
	}
	stopped := c.isInstanceTimeStopped(instanceID)

	var b strings.Builder
	instanceFrame(&b, inst, instTabWorld)

	if cc.ServerName() != inst.ProxyName {
		b.WriteString(box(0.3, 2.15, 10.4, 3.0, colorCard))
		b.WriteString(fmt.Sprintf("image[0.6,2.5;0.55,0.55;%s]", iconWarning))
		b.WriteString(coloredLbl(1.4, 2.8, light, "Time and weather are changed from inside the world."))
		if inst.Status == "running" {
			b.WriteString(hint(1.4, 3.3, "Join it, then open this tab again."))
			b.WriteString(styledBtn(1.4, 3.9, 3.5, 0.75, "btn_hop_me", "Go there", colorPrimary))
		} else {
			b.WriteString(hint(1.4, 3.3, "The world is off: start it from the Overview tab."))
		}
		cc.ShowFormspec("classrooms:world_controls", b.String())
		return
	}

	b.WriteString(box(0.3, 2.15, 10.4, 3.2, colorCard))
	b.WriteString(sectionTitle(0.55, 2.45, "Time of day"))
	b.WriteString(styledBtn(0.55, 2.85, 3.2, 1.1, "btn_world_day", "Day", "#b8892c"))
	b.WriteString(styledBtn(3.9, 2.85, 3.2, 1.1, "btn_world_evening", "Sunset", "#a3502c"))
	b.WriteString(styledBtn(7.25, 2.85, 3.2, 1.1, "btn_world_night", "Night", "#23325e"))
	if stopped {
		b.WriteString(fmt.Sprintf("image[0.55,4.3;0.45,0.45;%s]", iconWarning))
		b.WriteString(coloredLbl(1.15, 4.53, warning, "Time is paused."))
		b.WriteString(styledBtn(7.25, 4.2, 3.2, 0.75, "btn_world_resume_time", "Let time flow", colorActive))
	} else {
		b.WriteString(hint(0.55, 4.53, "Pause time to keep the light as it is."))
		b.WriteString(btn(7.25, 4.2, 3.2, 0.75, "btn_world_stop_time", "Pause time"))
	}

	b.WriteString(box(0.3, 5.55, 10.4, 2.2, colorCard))
	b.WriteString(sectionTitle(0.55, 5.85, "Weather"))
	b.WriteString(styledBtn(0.55, 6.25, 4.9, 1.1, "btn_world_sunny", "Clear sky", "#b8892c"))
	b.WriteString(styledBtn(5.55, 6.25, 4.9, 1.1, "btn_world_rain", "Rain", "#3a5f8f"))

	b.WriteString(hint(0.55, 8.2, "Changes apply to everyone in this world right away."))

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
	// A completely empty action is the explicit Resume button. Weather-only
	// actions must not also send the -1 sentinel as a time-of-day value.
	if shouldResumeWorldTime(preset, weather, stopTime) {
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
