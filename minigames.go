package main

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	proxy "github.com/HimbeerserverDE/mt-multiserver-proxy"
	"github.com/HimbeerserverDE/mt"
)

// ── HUB minigames ───────────────────────────────────────────────────────────
//
// The HUB runs the skyss_minigames mod; classrooms_bridge exposes it on the
// mod channel (minigame_list / minigame_start / minigame_stop, see
// docs/classrooms-plugin/bridge-mod.md). Messages reach the HUB through any
// player connected to it, and its replies reach the proxy once per player
// there, so every request carries a request_id and only the first reply
// counts.

const minigamesForm = "classrooms:minigames"

// stringList accepts a JSON array, or the object/null that Luanti's
// write_json produces for an empty Lua table.
type stringList []string

func (l *stringList) UnmarshalJSON(b []byte) error {
	var list []string
	if err := json.Unmarshal(b, &list); err == nil {
		*l = list
		return nil
	}
	*l = nil
	return nil
}

type minigameArena struct {
	Name       string     `json:"name"`
	Game       string     `json:"game"`
	Title      string     `json:"title"`
	Mode       string     `json:"mode"`
	MinPlayers int        `json:"min_players"`
	MaxPlayers int        `json:"max_players"`
	TimeLimit  int        `json:"time_limit"`
	State      string     `json:"state"`
	Players    stringList `json:"players"`
	Queue      stringList `json:"queue"`
}

func (a minigameArena) free() bool {
	return a.State == "free"
}

// minigameView is a teacher's Minigames dialog.
type minigameView struct {
	ClassID int
	Loaded  bool
	Arenas  []minigameArena
	Game    string // selected game
	Target  string // "class" or "group:<id>"
	Minutes string
	Message string
	Open    bool
}

var minigames = struct {
	sync.Mutex
	views   map[string]*minigameView // teacher -> dialog state
	pending map[string]string        // request_id -> teacher
}{
	views:   map[string]*minigameView{},
	pending: map[string]string{},
}

var minigameRequestSeq atomic.Uint64

func newMinigameRequest(teacher string) string {
	id := fmt.Sprintf("mg%d", minigameRequestSeq.Add(1))
	minigames.Lock()
	minigames.pending[id] = teacher
	minigames.Unlock()
	return id
}

// takeMinigameRequest returns the teacher of a pending request, once.
func takeMinigameRequest(id string) (string, bool) {
	minigames.Lock()
	defer minigames.Unlock()
	teacher, ok := minigames.pending[id]
	delete(minigames.pending, id)
	return teacher, ok
}

// hubConn is a connection on the HUB to send through: the teacher's own when
// they are there, else any player's.
func (c *controller) hubConn(prefer string) *proxy.ClientConn {
	if cc := proxy.Find(prefer); cc != nil && cc.ServerName() == c.cfg.LobbyServer {
		return cc
	}
	for name := range proxy.Players() {
		if cc := proxy.Find(name); cc != nil && cc.ServerName() == c.cfg.LobbyServer {
			return cc
		}
	}
	return nil
}

func (c *controller) sendToHub(teacher string, msg map[string]interface{}) bool {
	cc := c.hubConn(teacher)
	if cc == nil {
		return false
	}
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[%s] marshal error: %v", pluginName, err)
		return false
	}
	return cc.SendModChanMsg(modChannel, string(data))
}

func (c *controller) requestMinigameList(teacher string) bool {
	return c.sendToHub(teacher, map[string]interface{}{
		"action":     "minigame_list",
		"teacher":    teacher,
		"request_id": newMinigameRequest(teacher),
	})
}

// openMinigames shows the dialog for a class and asks the HUB for its arenas.
func (c *controller) openMinigames(cc *proxy.ClientConn, classID int) {
	minigames.Lock()
	view := minigames.views[cc.Name()]
	if view == nil || view.ClassID != classID {
		view = &minigameView{ClassID: classID, Target: "class"}
		minigames.views[cc.Name()] = view
	}
	view.Loaded = false
	view.Message = ""
	minigames.Unlock()

	if !c.requestMinigameList(cc.Name()) {
		minigames.Lock()
		view.Message = T("Nobody is in the HUB right now: minigames start there.")
		minigames.Unlock()
	}
	c.showMinigames(cc)
}

// handleMinigameReply handles minigame_list and minigame_result from the HUB.
func (c *controller) handleMinigameReply(cc *proxy.ClientConn, sender string, data map[string]interface{}) {
	if sender != "" || cc.ServerName() != c.cfg.LobbyServer {
		return
	}
	requestID, _ := data["request_id"].(string)
	teacher, ok := takeMinigameRequest(requestID)
	if !ok {
		return
	}
	action, _ := data["action"].(string)

	minigames.Lock()
	view := minigames.views[teacher]
	minigames.Unlock()
	tcc := proxy.Find(teacher)

	if action == "minigame_list" {
		raw, _ := json.Marshal(data["arenas"])
		var arenas []minigameArena
		if err := json.Unmarshal(raw, &arenas); err != nil {
			arenas = nil
		}
		sort.Slice(arenas, func(i, j int) bool { return arenas[i].Name < arenas[j].Name })
		if view == nil {
			return
		}
		minigames.Lock()
		view.Arenas = arenas
		view.Loaded = true
		open := view.Open
		minigames.Unlock()
		if open && tcc != nil {
			c.showMinigames(tcc)
		}
		return
	}

	// minigame_result
	okResult, _ := data["ok"].(bool)
	if request, _ := data["request"].(string); okResult && request == "start" && view != nil {
		minigames.Lock()
		view.Message = minigameResultText(data)
		minigames.Unlock()
	}
	if !okResult && tcc != nil {
		errText, _ := data["error"].(string)
		arena, _ := data["arena"].(string)
		if arena == "" {
			arena, _ = data["game"].(string)
		}
		msg := T("Minigame @1 did not start: @2", arena, errText)
		if request, _ := data["request"].(string); request == "stop" {
			msg = T("Minigame @1 could not be stopped: @2", arena, errText)
		}
		if view != nil {
			minigames.Lock()
			view.Message = msg
			minigames.Unlock()
		}
		// The bridge already told a teacher who is in the HUB.
		if tcc.ServerName() != c.cfg.LobbyServer {
			c.notify(tcc, msg)
		}
	}
	// Refresh the arena states shown in the dialog.
	c.requestMinigameList(teacher)
}

// minigameGames lists the games of the HUB once each, with their arenas.
func minigameGames(arenas []minigameArena) (keys []string, byGame map[string][]minigameArena) {
	byGame = map[string][]minigameArena{}
	for _, a := range arenas {
		if _, ok := byGame[a.Game]; !ok {
			keys = append(keys, a.Game)
		}
		byGame[a.Game] = append(byGame[a.Game], a)
	}
	return keys, byGame
}

// minigameTargets is the "who plays" dropdown: the class, then its groups.
func (c *controller) minigameTargets(classID int) (labels, keys []string, members map[string][]string) {
	members = map[string][]string{}
	students, _ := c.getStudents(classID)
	labels = append(labels, T("Whole class"))
	keys = append(keys, "class")
	members["class"] = students
	groups, _ := c.getGroups(classID)
	for _, g := range groups {
		key := "group:" + strconv.Itoa(g.ID)
		labels = append(labels, T("Group @1", g.Name))
		keys = append(keys, key)
		members[key] = g.Members
	}
	return labels, keys, members
}

// inHub keeps the names currently on the HUB server.
func (c *controller) inHub(names []string) []string {
	var out []string
	for _, n := range names {
		if cc := proxy.Find(n); cc != nil && cc.ServerName() == c.cfg.LobbyServer {
			out = append(out, n)
		}
	}
	return out
}

func minigameStateLabel(a minigameArena) string {
	switch a.State {
	case "free":
		return mcColorize(success, T("free"))
	case "resetting", "loading":
		return mcColorize(warning, T("getting ready"))
	default:
		return mcColorize(accent, T("playing (@1)", len(a.Players)))
	}
}

func (c *controller) showMinigames(cc *proxy.ClientConn) {
	minigames.Lock()
	view := minigames.views[cc.Name()]
	if view == nil {
		minigames.Unlock()
		return
	}
	view.Open = true
	v := *view
	minigames.Unlock()

	cls, err := c.getClassByID(v.ClassID)
	if err != nil || cls == nil {
		c.showMainDashboard(cc)
		return
	}

	var b strings.Builder
	fsOpen(&b, 14, 10)
	fsHeader(&b, 14, T("HUB minigames"), cls.Name, true, true)
	b.WriteString(iconBtn(12.4, 0.17, 0.66, "btn_refresh", iconRefresh, T("Refresh")))

	b.WriteString(box(0.2, 1.3, 13.6, 4.3, colorCard))
	b.WriteString(sectionTitle(0.45, 1.6, T("START A MINIGAME")))
	gameKeys, byGame := minigameGames(v.Arenas)
	switch {
	case !v.Loaded && v.Message == "":
		b.WriteString(hint(0.6, 2.4, T("Asking the HUB for its minigames...")))
	case v.Loaded && len(gameKeys) == 0:
		b.WriteString(hint(0.6, 2.4, T("The HUB has no minigames.")))
	}
	if len(gameKeys) > 0 {
		// indexOf falls back to the first entry.
		gameIdx := indexOf(gameKeys, v.Game)
		game := gameKeys[gameIdx-1]
		arenas := byGame[game]
		gameLabels := make([]string, len(gameKeys))
		for i, k := range gameKeys {
			gameLabels[i] = byGame[k][0].Title
		}
		targetLabels, targetKeys, members := c.minigameTargets(v.ClassID)
		targetIdx := indexOf(targetKeys, v.Target)
		players := c.inHub(members[targetKeys[targetIdx-1]])

		b.WriteString(coloredLbl(0.6, 2.25, muted, T("Minigame")))
		b.WriteString(fmt.Sprintf("dropdown[0.6,2.5;5.6,0.65;mg_game;%s;%d;true]", dropdownItems(gameLabels), gameIdx))
		b.WriteString(coloredLbl(6.5, 2.25, muted, T("Who plays")))
		b.WriteString(fmt.Sprintf("dropdown[6.5,2.5;4.4,0.65;mg_target;%s;%d;true]", dropdownItems(targetLabels), targetIdx))
		b.WriteString(coloredLbl(11.2, 2.25, muted, T("Minutes (0 = none)")))
		minutes := v.Minutes
		if minutes == "" {
			minutes = strconv.Itoa(arenas[0].TimeLimit / 60)
		}
		b.WriteString(fmt.Sprintf("field[11.2,2.5;2.3,0.65;mg_minutes;;%s]", fmtEsc(minutes)))
		b.WriteString("field_close_on_enter[mg_minutes;false]")

		a := arenas[0]
		var info string
		free := 0
		for _, ar := range arenas {
			if ar.free() {
				free++
			}
		}
		if a.Mode == "solo" {
			info = T("Solo: one student per arena. Free arenas: @1 of @2.", free, len(arenas))
		} else {
			info = T("Cooperative, @1-@2 players. Free arenas: @3 of @4.", a.MinPlayers, a.MaxPlayers, free, len(arenas))
		}
		b.WriteString(hint(0.6, 3.6, info))
		b.WriteString(hint(0.6, 4.05, T("@1 of these students are in the HUB now: only they will play.", len(players))))
		b.WriteString(styledBtn(9.6, 4.6, 3.9, 0.75, "btn_mg_start", T("Start"), colorPrimary))
	}
	if v.Message != "" {
		b.WriteString(coloredLbl(0.6, 4.95, warning, v.Message))
	}

	b.WriteString(box(0.2, 5.8, 13.6, 4.0, colorCard))
	b.WriteString(sectionTitle(0.45, 6.1, T("ARENAS")))
	b.WriteString("scroll_container[0.4,6.4;13.0,3.3;scr_mg;vertical;0.1]")
	y := 0.0
	for i, a := range v.Arenas {
		b.WriteString(box(0, y, 12.9, 0.62, colorRow))
		b.WriteString(fmt.Sprintf("label[0.2,%g;%s]", y+0.31, fmtEsc(mcColorize(light, a.Title)+mcColorize(muted, "  ("+a.Name+")"))))
		b.WriteString(fmt.Sprintf("label[6.0,%g;%s]", y+0.31, fmtEsc(minigameStateLabel(a))))
		if len(a.Players) > 0 {
			b.WriteString(fmt.Sprintf("label[8.0,%g;%s]", y+0.31, fmtEsc(mcColorize(muted, memberPreview(a.Players, 34)))))
		}
		if !a.free() && a.State != "resetting" {
			b.WriteString(btn(11.3, y+0.08, 1.5, 0.46, "mg_stop_"+strconv.Itoa(i), T("Stop")))
		}
		y += 0.7
	}
	b.WriteString("scroll_container_end[]")
	b.WriteString(scrollbarFor("scr_mg", 13.45, 6.4, 3.3, len(v.Arenas), 0.7, 0))

	cc.ShowFormspec(minigamesForm, b.String())
}

func (c *controller) handleMinigames(cc *proxy.ClientConn, fields []mt.Field) {
	name := cc.Name()
	fm := fieldMap(fields)
	minigames.Lock()
	view := minigames.views[name]
	if view != nil {
		if m, ok := fm["mg_minutes"]; ok {
			view.Minutes = strings.TrimSpace(m)
		}
	}
	minigames.Unlock()
	if view == nil {
		return
	}
	if !c.canManageClass(view.ClassID, name) {
		c.showMainDashboard(cc)
		return
	}
	_, closed := fm["quit"]
	_, closeBtn := fm["btn_close"]
	if closed || closeBtn {
		minigames.Lock()
		view.Open = false
		minigames.Unlock()
		return
	}
	if _, ok := fm["btn_back"]; ok {
		minigames.Lock()
		view.Open = false
		minigames.Unlock()
		c.showClassView(cc, view.ClassID)
		return
	}

	minigames.Lock()
	gameKeys, _ := minigameGames(view.Arenas)
	if idx, err := strconv.Atoi(fm["mg_game"]); err == nil && idx >= 1 && idx <= len(gameKeys) {
		if gameKeys[idx-1] != view.Game {
			view.Game = gameKeys[idx-1]
			view.Minutes = ""
		}
	}
	_, targetKeys, members := c.minigameTargets(view.ClassID)
	if idx, err := strconv.Atoi(fm["mg_target"]); err == nil && idx >= 1 && idx <= len(targetKeys) {
		view.Target = targetKeys[idx-1]
	}
	view.Message = ""
	v := *view
	minigames.Unlock()

	if _, ok := fm["btn_refresh"]; ok {
		c.openMinigames(cc, v.ClassID)
		return
	}
	for key := range fm {
		if !strings.HasPrefix(key, "mg_stop_") {
			continue
		}
		i, err := strconv.Atoi(strings.TrimPrefix(key, "mg_stop_"))
		if err != nil || i < 0 || i >= len(v.Arenas) {
			break
		}
		c.sendToHub(name, map[string]interface{}{
			"action":     "minigame_stop",
			"arena":      v.Arenas[i].Name,
			"teacher":    name,
			"request_id": newMinigameRequest(name),
		})
		c.showMinigames(cc)
		return
	}

	if _, ok := fm["btn_mg_start"]; ok && len(gameKeys) > 0 {
		game := gameKeys[indexOf(gameKeys, v.Game)-1]
		target := targetKeys[indexOf(targetKeys, v.Target)-1]
		msg := c.startMinigame(name, game, c.inHub(members[target]), v.Minutes)
		minigames.Lock()
		view.Message = msg
		minigames.Unlock()
	}
	c.showMinigames(cc)
}

// startMinigame asks the HUB to start the game for these players: solo
// games spread them over the free arenas and put the rest in line,
// cooperative games use the first free arena. The outcome arrives as
// minigame_result.
func (c *controller) startMinigame(teacher, game string, players []string, minutes string) string {
	if len(players) == 0 {
		return T("None of these students is in the HUB.")
	}
	msg := map[string]interface{}{
		"action":     "minigame_start",
		"game":       game,
		"players":    players,
		"teacher":    teacher,
		"request_id": newMinigameRequest(teacher),
	}
	if m, err := strconv.ParseFloat(strings.ReplaceAll(minutes, ",", "."), 64); err == nil && m >= 0 && m <= 120 {
		msg["time_limit"] = int(m*60 + 0.5)
	}
	if !c.sendToHub(teacher, msg) {
		return T("Nobody is in the HUB right now: minigames start there.")
	}
	return T("Starting...")
}

// minigameResultText describes a successful minigame_start.
func minigameResultText(data map[string]interface{}) string {
	started, _ := numberField(data, "started")
	waiting, _ := numberField(data, "waiting")
	leftOut, _ := numberField(data, "left_out")
	switch {
	case waiting > 0:
		return T("Started for @1 students; @2 are in line for a free arena.", int(started), int(waiting))
	case leftOut > 0:
		return T("Started for @1 students; @2 did not fit.", int(started), int(leftOut))
	}
	return T("Started for @1 students.", int(started))
}
