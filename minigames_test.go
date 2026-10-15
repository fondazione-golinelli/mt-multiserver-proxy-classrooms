package main

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestMinigameListDecodesLuaEmptyTables(t *testing.T) {
	// write_json turns an empty Lua table into {} or null.
	raw := `[{"name":"farm_1","game":"skyss_animalgame","mode":"solo","min_players":1,"max_players":1,
		"time_limit":0,"state":"free","players":{},"queue":null},
		{"name":"beach","game":"skyss_cleanup","state":"running","players":["anna","bob"],"queue":[]}]`
	var arenas []minigameArena
	if err := json.Unmarshal([]byte(raw), &arenas); err != nil {
		t.Fatal(err)
	}
	if len(arenas) != 2 || len(arenas[0].Players) != 0 || len(arenas[0].Queue) != 0 {
		t.Fatalf("empty lists not decoded: %+v", arenas)
	}
	if !reflect.DeepEqual([]string(arenas[1].Players), []string{"anna", "bob"}) {
		t.Fatalf("players = %v", arenas[1].Players)
	}
	if !arenas[0].free() || arenas[1].free() {
		t.Fatal("free() wrong")
	}
}

func TestMinigameGamesGroupsArenasInOrder(t *testing.T) {
	arenas := []minigameArena{
		{Name: "beach", Game: "skyss_cleanup"},
		{Name: "farm_1", Game: "skyss_animalgame"},
		{Name: "farm_2", Game: "skyss_animalgame"},
	}
	keys, byGame := minigameGames(arenas)
	if !reflect.DeepEqual(keys, []string{"skyss_cleanup", "skyss_animalgame"}) {
		t.Fatalf("keys = %v", keys)
	}
	if len(byGame["skyss_animalgame"]) != 2 || byGame["skyss_animalgame"][1].Name != "farm_2" {
		t.Fatalf("byGame = %v", byGame)
	}
}

func TestMinigameRequestIsTakenOnce(t *testing.T) {
	id := newMinigameRequest("teacher")
	if who, ok := takeMinigameRequest(id); !ok || who != "teacher" {
		t.Fatalf("first take = %q %v", who, ok)
	}
	if _, ok := takeMinigameRequest(id); ok {
		t.Fatal("a reply must be handled once")
	}
}
