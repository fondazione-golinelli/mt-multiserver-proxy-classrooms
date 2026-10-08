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
	p := missionPayload(&missionData{ID: 3, Title: "Farm", Goals: []missionGoal{{Type: "blocks", Key: "water", Count: 20}}}, nil)
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
