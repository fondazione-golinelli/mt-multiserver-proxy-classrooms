package main

import (
	"encoding/json"
)

// ── Special missions ────────────────────────────────────────────────────────
//
// A special mission is a mission with an activity of the bridge (or of a mod
// registered with it): its own rules, set up in the world when it starts,
// such as fossils hidden underground. The bridge describes the activities it
// offers next to the catalog ("special" in mission_catalog); the proxy stores
// the chosen kind and the teacher's settings with the mission and turns the
// activity's goals into ordinary goals of type "special", counted by the
// bridge like the others.

const goalSpecial = "special"

type specialSetting struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Type    string   `json:"type"` // "number" or "choice" (Options, 1-based)
	Default int      `json:"default"`
	Min     int      `json:"min"`
	Max     int      `json:"max"`
	Options []string `json:"options,omitempty"`
}

type specialGoal struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Icon      string `json:"icon,omitempty"`
	CountFrom string `json:"count_from"` // setting giving the amount
}

type specialKind struct {
	Key         string           `json:"key"`
	Label       string           `json:"label"`
	Description string           `json:"description"`
	Icon        string           `json:"icon,omitempty"`
	ZoneOnly    bool             `json:"zone_only"` // needs a zone: not offered for world missions
	Dig         bool             `json:"dig"`       // students dig: the zone must let them build
	Settings    []specialSetting `json:"settings"`
	Goals       []specialGoal    `json:"goals"`
	Tools       []catalogEntry   `json:"tools"` // handed out automatically in the zone
}

// clamp keeps a setting value within its bounds.
func (s specialSetting) clamp(v int) int {
	lo, hi := s.Min, s.Max
	if s.Type == "choice" {
		lo, hi = 1, len(s.Options)
	}
	if hi < lo {
		return lo
	}
	return min(max(v, lo), hi)
}

func (c *controller) getSpecials(instanceID string) []specialKind {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.runtime.missionSpecials[instanceID]
}

func findSpecial(kinds []specialKind, key string) *specialKind {
	for i := range kinds {
		if kinds[i].Key == key {
			return &kinds[i]
		}
	}
	return nil
}

// parseSpecials reads the activities of a mission_catalog message.
func parseSpecials(v interface{}) []specialKind {
	raw, _ := json.Marshal(v)
	var kinds []specialKind
	if err := json.Unmarshal(raw, &kinds); err != nil {
		return nil
	}
	out := kinds[:0]
	for _, k := range kinds {
		if k.Key != "" {
			out = append(out, k)
		}
	}
	return out
}

// setSpecial makes the draft a special mission of kind k (nil: a normal
// mission again), with default settings and the activity's goals first.
func (d *missionDraft) setSpecial(k *specialKind) {
	d.Kind, d.Settings = "", nil
	if k != nil {
		d.Kind, d.Settings = k.Key, map[string]int{}
		for _, s := range k.Settings {
			d.Settings[s.Key] = s.clamp(s.Default)
		}
	}
	d.syncSpecialGoals(k)
}

// syncSpecialGoals rebuilds the activity's goals from the settings; goals
// whose amount is 0 are left out. Goals added by the teacher stay after them.
func (d *missionDraft) syncSpecialGoals(k *specialKind) {
	var goals []missionGoal
	if k != nil {
		for _, g := range k.Goals {
			if n := d.Settings[g.CountFrom]; n > 0 {
				goals = append(goals, missionGoal{Type: goalSpecial, Key: g.Key, Label: g.Label, Icon: g.Icon, Count: n})
			}
		}
	}
	for _, g := range d.Goals {
		if g.Type != goalSpecial {
			goals = append(goals, g)
		}
	}
	if len(goals) > maxMissionGoals {
		goals = goals[:maxMissionGoals]
	}
	d.Goals = goals
}

// maxTeacherTools is how many kinds of tools the teacher may add next to the
// ones the activity hands out.
func maxTeacherTools(k *specialKind) int {
	if k == nil {
		return maxMissionTools
	}
	return max(maxMissionTools-len(k.Tools), 0)
}

func findTool(tools []missionTool, key string) *missionTool {
	for i := range tools {
		if tools[i].Key == key {
			return &tools[i]
		}
	}
	return nil
}

// activeTools lists the tools the activity hands out with these settings.
func (k *specialKind) activeTools(settings map[string]int) []catalogEntry {
	var out []catalogEntry
	for _, t := range k.Tools {
		if t.Setting == "" || settings[t.Setting] >= t.Min {
			out = append(out, t)
		}
	}
	return out
}
