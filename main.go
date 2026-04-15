package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	proxy "github.com/HimbeerserverDE/mt-multiserver-proxy"
)

const pluginName = "classrooms"
const modChannel = "classrooms:cmd"

// ── Config ──────────────────────────────────────────────────────────────────

const (
	defaultPollIntervalSeconds = 2
	defaultPollTimeoutSeconds  = 180
	defaultStartGraceSeconds   = 2
	defaultJoinRetryCount      = 12
	defaultJoinRetryDelayMS    = 1000
	defaultInternalPort        = 30000
)

type classroomsConfig struct {
	// Pelican API
	PanelURL             string `json:"panel_url"`
	ApplicationToken     string `json:"application_api_token"`
	ApplicationTokenFile string `json:"application_api_token_file"`

	// Global Game
	DefaultGame string `json:"default_game"`

	// Lobby
	LobbyServer string `json:"lobby_server"`

	// Timing / polling
	PollIntervalSeconds  int `json:"poll_interval_seconds"`
	PollTimeoutSeconds   int `json:"poll_timeout_seconds"`
	StartGraceSeconds    int `json:"start_grace_seconds"`
	JoinRetryCount       int `json:"join_retry_count"`
	JoinRetryDelayMillis int `json:"join_retry_delay_millis"`

	// MySQL database
	DBHost         string `json:"db_host"`
	DBName         string `json:"db_name"`
	DBUser         string `json:"db_user"`
	DBPassword     string `json:"db_password"`
	DBPasswordFile string `json:"db_password_file"`

	// Templates
	Templates map[string]templateConfig `json:"templates"`
}

type templateConfig struct {
	UserID                int            `json:"user_id"`
	EggID                 int            `json:"egg_id"`
	MountID               int            `json:"mount_id"`
	TemplateName          string         `json:"template_name"`
	InstanceTemplateMount string         `json:"instance_template_mount"`
	WorldName             string         `json:"world_name"`
	AdminName             string         `json:"admin_name"`
	NamePrefix            string         `json:"name_prefix"`
	ServerDescription     string         `json:"server_description"`
	ServerDomain          string         `json:"server_domain"`
	ServerURL             string         `json:"server_url"`
	ServerAnnounce        bool           `json:"server_announce"`
	ServerListURL         string         `json:"server_list_url"`
	ServerMaxUsers        string         `json:"server_max_users"`
	ServerMOTD            string         `json:"server_motd"`
	ServerPassword        string         `json:"server_password"`
	LocationIDs           []int          `json:"location_ids"`
	Tags                  []string       `json:"tags"`
	DockerImage           string         `json:"docker_image"`
	Startup               string         `json:"startup"`
	InternalPort          int            `json:"internal_port"`
	MediaPool             string         `json:"media_pool"`
	Groups                []string       `json:"groups"`
	Limits                resourceLimits `json:"limits"`
	FeatureLimits         featureLimits  `json:"feature_limits"`

	// Visibility
	Public bool `json:"public"`
}

type resourceLimits struct {
	Memory  int  `json:"memory"`
	Swap    int  `json:"swap"`
	Disk    int  `json:"disk"`
	IO      int  `json:"io"`
	CPU     int  `json:"cpu"`
	Threads *int `json:"threads"`
}

type featureLimits struct {
	Databases   int `json:"databases"`
	Allocations int `json:"allocations"`
	Backups     int `json:"backups"`
}

// ── Runtime State (not persisted) ───────────────────────────────────────────

type runtimeState struct {
	frozenPlayers   map[string]bool     // player -> frozen
	watchingPlayers map[string]string   // student -> teacher
	activeClass     map[string]int      // player -> class ID they have open
	activeInstance  map[string]string   // player -> instance ID they have open
	pendingOps      map[string]struct{} // player -> in-flight operation
}

func newRuntimeState() runtimeState {
	return runtimeState{
		frozenPlayers:   make(map[string]bool),
		watchingPlayers: make(map[string]string),
		activeClass:     make(map[string]int),
		activeInstance:  make(map[string]string),
		pendingOps:      make(map[string]struct{}),
	}
}

// ── Controller ──────────────────────────────────────────────────────────────

type controller struct {
	cfg        classroomsConfig
	db         *sql.DB
	runtime    runtimeState
	httpClient *http.Client

	mu sync.RWMutex // protects runtime state

	nodeMu    sync.Mutex
	nodeCache map[int]nodeEndpoint
}

var (
	ctrl     *controller
	ctrlOnce sync.Once
)

func init() {
	ctrlOnce.Do(func() {
		cfg, err := loadConfig()
		if err != nil {
			log.Printf("[%s] disabled: %v", pluginName, err)
			return
		}

		db, err := openDB(cfg)
		if err != nil {
			log.Printf("[%s] disabled: database error: %v", pluginName, err)
			return
		}

		if err := migrateDB(db); err != nil {
			log.Printf("[%s] disabled: migration error: %v", pluginName, err)
			db.Close()
			return
		}

		c := &controller{
			cfg:        cfg,
			db:         db,
			runtime:    newRuntimeState(),
			httpClient: &http.Client{Timeout: 30 * time.Second},
			nodeCache:  make(map[int]nodeEndpoint),
		}

		ctrl = c

		// Startup reconciliation and background tasks
		c.reconcileInstances()
		c.startStatusChecker()

		// Register hooks
		proxy.RegisterOnChatMsg(c.onChatMsg)

		// Register commands
		c.registerCommands()

		// Register formspec handlers
		c.registerHandlers()

		// Register mod channel
		c.registerModChannel()

		// Register join/leave hooks
		c.registerJoinLeave()

		tCount, _ := c.countTeachers()
		cCount, _ := c.countClasses()
		iCount, _ := c.countInstances()
		log.Printf("[%s] loaded — %d teachers, %d classes, %d instances, %d templates",
			pluginName, tCount, cCount, iCount, len(cfg.Templates))
	})
}

// ── Config Loading ──────────────────────────────────────────────────────────

func loadConfig() (classroomsConfig, error) {
	path := strings.TrimSpace(os.Getenv("CLASSROOMS_CONFIG"))
	if path == "" {
		var ok bool
		path, ok = firstExistingPath(
			filepath.Join(proxy.Path("plugins"), pluginName, "config.json"),
			filepath.Join(proxy.Path("plugins"), pluginName+".json"),
			"config.json",
			pluginName+".json",
		)
		if !ok {
			return classroomsConfig{}, fmt.Errorf("no config file found in search paths")
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return classroomsConfig{}, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg classroomsConfig
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return classroomsConfig{}, fmt.Errorf("decode config %s: %w", path, err)
	}

	// Pelican API
	if cfg.PanelURL == "" {
		return classroomsConfig{}, errors.New("panel_url is required")
	}
	cfg.PanelURL = strings.TrimRight(cfg.PanelURL, "/")

	token := strings.TrimSpace(cfg.ApplicationToken)
	if token == "" && cfg.ApplicationTokenFile != "" {
		secret, err := os.ReadFile(cfg.ApplicationTokenFile)
		if err != nil {
			return classroomsConfig{}, fmt.Errorf("read application_api_token_file: %w", err)
		}
		token = strings.TrimSpace(string(secret))
	}
	if token == "" {
		return classroomsConfig{}, errors.New("application_api_token or application_api_token_file is required")
	}
	cfg.ApplicationToken = token

	// Database
	if cfg.DBHost == "" {
		return classroomsConfig{}, errors.New("db_host is required")
	}
	if cfg.DBName == "" {
		return classroomsConfig{}, errors.New("db_name is required")
	}
	if cfg.DBUser == "" {
		return classroomsConfig{}, errors.New("db_user is required")
	}

	if cfg.DefaultGame == "" {
		return classroomsConfig{}, errors.New("default_game is required")
	}
	if cfg.LobbyServer == "" {
		return classroomsConfig{}, errors.New("lobby_server is required")
	}

	dbPass := strings.TrimSpace(cfg.DBPassword)
	if dbPass == "" && cfg.DBPasswordFile != "" {
		secret, err := os.ReadFile(cfg.DBPasswordFile)
		if err != nil {
			return classroomsConfig{}, fmt.Errorf("read db_password_file: %w", err)
		}
		dbPass = strings.TrimSpace(string(secret))
	}
	cfg.DBPassword = dbPass

	// Defaults
	if cfg.PollIntervalSeconds <= 0 {
		cfg.PollIntervalSeconds = defaultPollIntervalSeconds
	}
	if cfg.PollTimeoutSeconds <= 0 {
		cfg.PollTimeoutSeconds = defaultPollTimeoutSeconds
	}
	if cfg.StartGraceSeconds <= 0 {
		cfg.StartGraceSeconds = defaultStartGraceSeconds
	}
	if cfg.JoinRetryCount <= 0 {
		cfg.JoinRetryCount = defaultJoinRetryCount
	}
	if cfg.JoinRetryDelayMillis <= 0 {
		cfg.JoinRetryDelayMillis = defaultJoinRetryDelayMS
	}

	// Templates
	if len(cfg.Templates) == 0 {
		return classroomsConfig{}, errors.New("at least one template must be configured")
	}
	for key, tpl := range cfg.Templates {
		if err := validateTemplate(key, &tpl); err != nil {
			return classroomsConfig{}, err
		}
		cfg.Templates[key] = tpl
	}

	log.Printf("[%s] config loaded from %s", pluginName, path)
	return cfg, nil
}

func validateTemplate(name string, tpl *templateConfig) error {
	if tpl.UserID <= 0 {
		return fmt.Errorf("template %q: user_id must be > 0", name)
	}
	if tpl.EggID <= 0 {
		return fmt.Errorf("template %q: egg_id must be > 0", name)
	}
	if tpl.MountID <= 0 {
		return fmt.Errorf("template %q: mount_id must be > 0", name)
	}
	if tpl.TemplateName == "" {
		return fmt.Errorf("template %q: template_name is required", name)
	}
	if tpl.InstanceTemplateMount == "" {
		tpl.InstanceTemplateMount = "/home/mount"
	}
	if tpl.WorldName == "" {
		tpl.WorldName = "world"
	}
	if len(tpl.LocationIDs) == 0 {
		return fmt.Errorf("template %q: location_ids must not be empty", name)
	}
	if tpl.NamePrefix == "" {
		tpl.NamePrefix = name
	}
	if tpl.InternalPort <= 0 {
		tpl.InternalPort = defaultInternalPort
	}
	if tpl.MediaPool == "" {
		return fmt.Errorf("template %q: media_pool is required", name)
	}
	if tpl.ServerDescription == "" {
		tpl.ServerDescription = "Dynamic classroom instance"
	}
	if tpl.ServerDomain == "" {
		tpl.ServerDomain = "internal.luanti"
	}
	if tpl.ServerURL == "" {
		tpl.ServerURL = "https://www.luanti.org"
	}
	if tpl.ServerListURL == "" {
		tpl.ServerListURL = "servers.luanti.org"
	}
	if tpl.ServerMaxUsers == "" {
		tpl.ServerMaxUsers = "15"
	}
	if tpl.Limits.IO == 0 {
		tpl.Limits.IO = 500
	}
	return nil
}

// ── Shared Helpers ──────────────────────────────────────────────────────────

func firstExistingPath(paths ...string) (string, bool) {
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	}
	return "", false
}

// isTeacher checks if a player is a registered teacher or has server perms.
func (c *controller) isTeacher(name string) bool {
	if ok, _ := c.getTeacher(name); ok {
		return true
	}
	cc := proxy.Find(name)
	return cc != nil && cc.HasPerms("server")
}

// isAdmin checks if a player has server-level proxy permissions.
func (c *controller) isAdmin(name string) bool {
	cc := proxy.Find(name)
	return cc != nil && cc.HasPerms("server")
}

// sendToPlayerServer sends a mod channel message to the backend server
// that a specific player is currently connected to.
func (c *controller) sendToPlayerServer(playerName string, msg interface{}) {
	cc := proxy.Find(playerName)
	if cc == nil {
		return
	}
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[%s] marshal error: %v", pluginName, err)
		return
	}
	cc.SendModChanMsg(modChannel, string(data))
}

// getOnlinePlayers returns all player names currently connected to the proxy.
func getOnlinePlayers() []string {
	players := proxy.Players()
	result := make([]string, 0, len(players))
	for name := range players {
		result = append(result, name)
	}
	return result
}

// getPlayerServer returns the server name a player is on, or "" if offline.
func getPlayerServer(name string) string {
	cc := proxy.Find(name)
	if cc == nil {
		return ""
	}
	return cc.ServerName()
}

// beginOp marks a player as having an in-flight operation. Returns false if busy.
func (c *controller) beginOp(player string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.runtime.pendingOps[player]; ok {
		return false
	}
	c.runtime.pendingOps[player] = struct{}{}
	return true
}

// endOp clears the in-flight operation for a player.
func (c *controller) endOp(player string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.runtime.pendingOps, player)
}
