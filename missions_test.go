package main

import "testing"

func TestMissionHelpers(t *testing.T) {
	if got := parseAmount(" 20 ", 1, 999); got != 20 {
		t.Fatalf("parseAmount = %d", got)
	}
	if parseAmount("abc", 7, 99) != 7 || parseAmount("0", 7, 99) != 7 || parseAmount("5000", 1, 999) != 999 {
		t.Fatal("parseAmount fallback/clamp")
	}
	if goalText(missionGoal{Type: "deliver", Label: "Wheat", Count: 20}) != "Deliver 20 Wheat" {
		t.Fatal("deliver goal text")
	}
	if goalText(missionGoal{Type: "animals", Label: "Cow", Count: 4}) != "4 × Cow in the zone" {
		t.Fatal("animals goal text")
	}
	if toolsText(nil) != "none" || toolsText([]missionTool{{Label: "Hoe", Count: 1}, {Label: "Seeds", Count: 16}}) != "Hoe ×1, Seeds ×16" {
		t.Fatal("tools text")
	}
	p := missionPayload(&missionData{ID: 3, Title: "Farm", Goals: []missionGoal{{Type: "blocks", Key: "water", Count: 20}}}, nil, "#e05252")
	if p["id"] != 3 || len(p["participants"].([]string)) != 0 || len(p["objectives"].([]map[string]interface{})) != 1 {
		t.Fatalf("payload %v", p)
	}
	if goalText(missionGoal{Type: "blocks", Label: "Water", Count: 20}) != "20 × Water blocks in the zone" {
		t.Fatal("blocks goal text")
	}
	if clampIndex(0, 3) != 1 || clampIndex(4, 3) != 1 || clampIndex(2, 3) != 2 {
		t.Fatal("clampIndex")
	}
}

func TestMissionMerging(t *testing.T) {
	tools, _ := addTool(nil, missionTool{Key: "sapling", Count: 3}, 99)
	tools, _ = addTool(tools, missionTool{Key: "sapling", Count: 10}, 99)
	tools, _ = addTool(tools, missionTool{Key: "hoe", Count: 1}, 99)
	if len(tools) != 2 || tools[0].Count != 13 {
		t.Fatalf("tools not merged: %+v", tools)
	}
	tools, _ = addTool(tools, missionTool{Key: "sapling", Count: 95}, 99)
	if tools[0].Count != 99 {
		t.Fatalf("tool amount not capped: %d", tools[0].Count)
	}
	goals, _ := addGoal(nil, missionGoal{Type: "deliver", Key: "wheat", Count: 5}, 999)
	goals, _ = addGoal(goals, missionGoal{Type: "deliver", Key: "wheat", Count: 15}, 999)
	goals, _ = addGoal(goals, missionGoal{Type: "blocks", Key: "wheat", Count: 1}, 999)
	if len(goals) != 2 || goals[0].Count != 20 {
		t.Fatalf("goals not merged by type and key: %+v", goals)
	}
	for i := 0; i < maxMissionGoals; i++ {
		goals, _ = addGoal(goals, missionGoal{Type: "animals", Key: string(rune('a' + i)), Count: 1}, 999)
	}
	if _, ok := addGoal(goals, missionGoal{Type: "animals", Key: "zz", Count: 1}, 999); ok || len(goals) != maxMissionGoals {
		t.Fatalf("goal limit not enforced: %d", len(goals))
	}
}

func TestWorldMissionHelpers(t *testing.T) {
	zones := []zoneData{{ID: 1, Name: "Farm"}, {ID: 2, Name: "Lake"}}
	labels, ids := newMissionPlaces(zones, map[int]*missionData{1: {ID: 9, ZoneID: 1}})
	if len(labels) != 2 || labels[0] != "Whole world" || labels[1] != "Zone: Lake" || ids[0] != 0 || ids[1] != 2 {
		t.Fatalf("places: %v %v", labels, ids)
	}
	animals := []missionGoal{{Type: "animals", Label: "Cow", Count: 4}}
	if needsChest(animals, false) || !needsChest(animals, true) {
		t.Fatal("world missions count animals around the chest")
	}
	collect := []missionGoal{{Type: "collect", Label: "Coal", Count: 5}}
	if needsChest(collect, true) || !needsChest([]missionGoal{{Type: "deliver"}}, false) {
		t.Fatal("gathering needs no chest, deliveries do")
	}
	if got := goalTextFor(animals[0], true); got != "4 × Cow near the chest" {
		t.Fatalf("world goal text: %q", got)
	}
	if got := goalTextFor(collect[0], false); got != "Gather 5 Coal" {
		t.Fatalf("collect text: %q", got)
	}
	if catalogKind("collect") != "deliver" || catalogKind("blocks") != "blocks" {
		t.Fatal("gather goals pick from the item catalog")
	}
	m := &missionData{ID: 3, Title: "Mine", Goals: collect}
	if p := missionPayload(m, nil, worldMissionColor); p["global"] != true {
		t.Fatalf("world mission payload: %v", p)
	}
	d := draftFromMission(&missionData{ID: 7, GroupID: 4, Title: "T", Goals: collect})
	if d.EditingID != 7 || d.GroupID != 4 || !d.global() || &d.Goals[0] == &collect[0] {
		t.Fatalf("draft from mission: %+v", d)
	}
}
