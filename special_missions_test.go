package main

import "testing"

func TestSpecialMissions(t *testing.T) {
	kinds := parseSpecials([]interface{}{
		map[string]interface{}{"key": ""},
		map[string]interface{}{
			"key": "fossils", "label": "Find the fossils", "zone_only": true, "dig": true,
			"settings": []interface{}{
				map[string]interface{}{"key": "small", "type": "number", "default": 10, "min": 0, "max": 40},
				map[string]interface{}{"key": "skeletons", "type": "number", "default": 1, "min": 0, "max": 4},
				map[string]interface{}{"key": "depth", "type": "choice", "default": 9, "options": []string{"a", "b", "c"}},
			},
			"goals": []interface{}{
				map[string]interface{}{"key": "fossils_small", "label": "Fossils found", "count_from": "small"},
				map[string]interface{}{"key": "fossils_skeletons", "label": "Skeletons", "count_from": "skeletons"},
			},
			"tools": []interface{}{map[string]interface{}{"key": "hammer"}, map[string]interface{}{"key": "brush"}},
		},
	})
	if len(kinds) != 1 || !kinds[0].ZoneOnly || len(kinds[0].Settings) != 3 {
		t.Fatalf("parseSpecials = %+v", kinds)
	}
	k := &kinds[0]

	d := newMissionDraft(5)
	d.Goals = []missionGoal{{Type: "deliver", Key: "wheat", Count: 3}}
	d.setSpecial(k)
	if d.Kind != "fossils" || d.Settings["depth"] != 3 || d.Settings["small"] != 10 {
		t.Fatalf("defaults %+v", d.Settings)
	}
	if len(d.Goals) != 3 || d.Goals[0].Type != goalSpecial || d.Goals[0].Count != 10 || d.Goals[2].Key != "wheat" {
		t.Fatalf("goals %+v", d.Goals)
	}
	// A goal with amount 0 is left out; the teacher's goals stay.
	d.Settings["skeletons"] = 0
	d.syncSpecialGoals(k)
	if len(d.Goals) != 2 || d.Goals[1].Key != "wheat" {
		t.Fatalf("goals without skeletons %+v", d.Goals)
	}
	d.setSpecial(nil)
	if d.Kind != "" || len(d.Goals) != 1 || d.Goals[0].Key != "wheat" {
		t.Fatalf("normal again %+v", d.Goals)
	}
	if maxTeacherTools(k) != maxMissionTools-len(k.Tools) || maxTeacherTools(nil) != maxMissionTools {
		t.Fatal("maxTeacherTools")
	}
	k.Tools = append(k.Tools, catalogEntry{Key: "detector", Setting: "help", Min: 3})
	if n := len(k.activeTools(map[string]int{"help": 2})); n != 2 {
		t.Fatalf("activeTools help 2 = %d", n)
	}
	if n := len(k.activeTools(map[string]int{"help": 3})); n != 3 {
		t.Fatalf("activeTools help 3 = %d", n)
	}
	if (specialSetting{Type: "number", Min: 0, Max: 40}).clamp(99) != 40 {
		t.Fatal("clamp")
	}

	m := &missionData{ID: 4, ZoneID: 5, Kind: "fossils", Settings: map[string]int{"small": 12}}
	p := missionPayload(m, nil, "#fff")
	if p["kind"] != "fossils" || p["settings"].(map[string]int)["small"] != 12 {
		t.Fatalf("payload %v", p)
	}
	if _, ok := missionPayload(&missionData{ID: 5}, nil, "#fff")["kind"]; ok {
		t.Fatal("normal mission has no kind")
	}
	if goalText(missionGoal{Type: goalSpecial, Label: "Fossils found", Count: 12}) != T("@1: @2", "Fossils found", 12) {
		t.Fatal("special goal text")
	}
}
