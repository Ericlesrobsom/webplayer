package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// ========== CONFIG (.env) ==========

type Config struct {
	ServerURL    string
	Port         string
	PlayerName   string
	LogoPath     string
	SyncUser     string
	SyncPass     string
	SyncInterval int
	ItemsPerPage int
	Accent       string
	AccentLight  string
}

var cfg Config

func loadEnv() {
	cfg = Config{
		Port:         "8080",
		PlayerName:   "IPTV Player",
		SyncInterval: 6,
		ItemsPerPage: 100,
	}

	data, err := os.ReadFile(".env")
	if err != nil {
		log.Println("⚠️ .env não encontrado, usando padrões")
		return
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(strings.ToUpper(parts[0]))
		val := strings.TrimSpace(parts[1])

		switch key {
		case "SERVER_URL":
			cfg.ServerURL = strings.TrimRight(val, "/")
		case "PORT":
			cfg.Port = val
		case "PLAYER_NAME":
			cfg.PlayerName = val
		case "LOGO":
			cfg.LogoPath = val
		case "SYNC_USER":
			cfg.SyncUser = val
		case "SYNC_PASS":
			cfg.SyncPass = val
		case "SYNC_INTERVAL":
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				cfg.SyncInterval = n
			}
		case "ITEMS_PER_PAGE":
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				cfg.ItemsPerPage = n
			}
		case "ACCENT", "THEME", "COR":
			parts := strings.SplitN(val, ",", 2)
			cfg.Accent = strings.TrimSpace(parts[0])
			if len(parts) > 1 {
				cfg.AccentLight = strings.TrimSpace(parts[1])
			}
		case "ACCENT_LIGHT":
			cfg.AccentLight = val
		}
	}
}

// ========== DATABASE ==========

var db *sql.DB

func initDB() {
	var err error
	db, err = sql.Open("sqlite3", "./webplayer.db?_journal_mode=WAL&_busy_timeout=5000&cache=shared")
	if err != nil {
		log.Fatal("❌ Erro SQLite:", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS categories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			category_id TEXT NOT NULL,
			category_name TEXT NOT NULL,
			type TEXT NOT NULL,
			data_json TEXT,
			UNIQUE(category_id, type)
		);
		CREATE TABLE IF NOT EXISTS streams (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			stream_id TEXT NOT NULL,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			category_id TEXT,
			icon TEXT,
			rating TEXT,
			added TEXT,
			data_json TEXT NOT NULL,
			UNIQUE(stream_id, type)
		);
		CREATE TABLE IF NOT EXISTS user_favorites (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL,
			item_type TEXT NOT NULL,
			item_id TEXT NOT NULL,
			name TEXT,
			img TEXT,
			created_at INTEGER,
			UNIQUE(username, item_type, item_id)
		);
		CREATE TABLE IF NOT EXISTS user_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL,
			item_type TEXT NOT NULL,
			item_id TEXT NOT NULL,
			parent_id TEXT DEFAULT '',
			name TEXT,
			img TEXT,
			season TEXT DEFAULT '',
			episode TEXT DEFAULT '',
			position_sec REAL DEFAULT 0,
			duration_sec REAL DEFAULT 0,
			updated_at INTEGER,
			UNIQUE(username, item_type, item_id)
		);
		CREATE TABLE IF NOT EXISTS sync_meta (
			key TEXT PRIMARY KEY,
			value TEXT,
			updated_at INTEGER
		);
		CREATE INDEX IF NOT EXISTS idx_streams_type ON streams(type);
		CREATE INDEX IF NOT EXISTS idx_streams_cat ON streams(category_id, type);
		CREATE INDEX IF NOT EXISTS idx_favs_user ON user_favorites(username);
		CREATE INDEX IF NOT EXISTS idx_hist_user ON user_history(username);
	`)
	if err != nil {
		log.Fatal("❌ Erro criando tabelas:", err)
	}

	// Migração para bancos existentes
	db.Exec("ALTER TABLE user_history ADD COLUMN parent_id TEXT DEFAULT ''")
}

// ========== HTTP CLIENT ==========

var httpClient = &http.Client{Timeout: 30 * time.Second}

func serverURL() string { return cfg.ServerURL }

// ========== SESSIONS ==========

type Session struct {
	Username  string
	Password  string
	UserInfo  json.RawMessage
	CreatedAt time.Time
}

var (
	sessions = make(map[string]*Session)
	sessMu   sync.RWMutex
)

func genToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func getSession(r *http.Request) *Session {
	c, err := r.Cookie("session_token")
	if err != nil {
		return nil
	}
	sessMu.RLock()
	defer sessMu.RUnlock()
	return sessions[c.Value]
}

// ========== SYNC ENGINE ==========

func syncAll() {
	if cfg.SyncUser == "" || cfg.SyncPass == "" || cfg.ServerURL == "" {
		log.Println("⚠️ Sync: faltam SYNC_USER, SYNC_PASS ou SERVER_URL no .env")
		return
	}
	log.Println("🔄 Iniciando sincronização...")
	start := time.Now()

	// Sync categories
	syncCategories("get_live_categories", "live")
	syncCategories("get_vod_categories", "movie")
	syncCategories("get_series_categories", "series")

	// Sync streams
	syncStreams("get_live_streams", "live")
	syncStreams("get_vod_streams", "movie")
	syncStreams("get_series", "series")

	db.Exec("INSERT OR REPLACE INTO sync_meta (key, value, updated_at) VALUES ('last_sync', ?, ?)",
		time.Now().Format("2006-01-02 15:04:05"), time.Now().Unix())

	log.Printf("✅ Sincronização completa em %v", time.Since(start).Round(time.Millisecond))
}

func syncCategories(action, contentType string) {
	apiURL := fmt.Sprintf("%s/player_api.php?username=%s&password=%s&action=%s",
		cfg.ServerURL, url.QueryEscape(cfg.SyncUser), url.QueryEscape(cfg.SyncPass), action)

	resp, err := httpClient.Get(apiURL)
	if err != nil {
		log.Printf("❌ Sync %s: %v", action, err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var cats []map[string]interface{}
	if err := json.Unmarshal(body, &cats); err != nil {
		log.Printf("❌ Sync %s parse: %v", action, err)
		return
	}

	tx, _ := db.Begin()
	tx.Exec("DELETE FROM categories WHERE type = ?", contentType)
	stmt, _ := tx.Prepare("INSERT INTO categories (category_id, category_name, type, data_json) VALUES (?, ?, ?, ?)")
	for _, cat := range cats {
		catID := fmt.Sprintf("%v", cat["category_id"])
		catName, _ := cat["category_name"].(string)
		jdata, _ := json.Marshal(cat)
		stmt.Exec(catID, catName, contentType, string(jdata))
	}
	stmt.Close()
	tx.Commit()
	log.Printf("   ✅ %s: %d categorias", action, len(cats))
}

func syncStreams(action, contentType string) {
	apiURL := fmt.Sprintf("%s/player_api.php?username=%s&password=%s&action=%s",
		cfg.ServerURL, url.QueryEscape(cfg.SyncUser), url.QueryEscape(cfg.SyncPass), action)

	resp, err := httpClient.Get(apiURL)
	if err != nil {
		log.Printf("❌ Sync %s: %v", action, err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var items []json.RawMessage
	if err := json.Unmarshal(body, &items); err != nil {
		log.Printf("❌ Sync %s parse: %v", action, err)
		return
	}

	tx, _ := db.Begin()
	tx.Exec("DELETE FROM streams WHERE type = ?", contentType)
	stmt, _ := tx.Prepare("INSERT INTO streams (stream_id, name, type, category_id, icon, rating, added, data_json) VALUES (?, ?, ?, ?, ?, ?, ?, ?)")

	for _, raw := range items {
		var peek struct {
			StreamID interface{} `json:"stream_id"`
			SeriesID interface{} `json:"series_id"`
			Name     string      `json:"name"`
			CatID    interface{} `json:"category_id"`
			Icon     string      `json:"stream_icon"`
			Cover    string      `json:"cover"`
			Rating   interface{} `json:"rating"`
			Added    string      `json:"added"`
		}
		json.Unmarshal(raw, &peek)

		sid := fmt.Sprintf("%v", peek.StreamID)
		if contentType == "series" {
			sid = fmt.Sprintf("%v", peek.SeriesID)
		}
		catID := fmt.Sprintf("%v", peek.CatID)
		icon := peek.Icon
		if icon == "" {
			icon = peek.Cover
		}
		rating := fmt.Sprintf("%v", peek.Rating)
		if rating == "<nil>" {
			rating = ""
		}

		stmt.Exec(sid, peek.Name, contentType, catID, icon, rating, peek.Added, string(raw))
	}
	stmt.Close()
	tx.Commit()
	log.Printf("   ✅ %s: %d itens", action, len(items))
}

func startSyncLoop() {
	syncAll()
	interval := time.Duration(cfg.SyncInterval) * time.Hour
	for {
		time.Sleep(interval)
		syncAll()
	}
}

// ========== API HANDLERS ==========

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", 405)
		return
	}

	var creds struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		jsonErr(w, "Invalid request", 400)
		return
	}
	if serverURL() == "" {
		jsonErr(w, "Servidor não configurado no .env", 500)
		return
	}

	apiURL := fmt.Sprintf("%s/player_api.php?username=%s&password=%s",
		serverURL(), url.QueryEscape(creds.Username), url.QueryEscape(creds.Password))

	resp, err := httpClient.Get(apiURL)
	if err != nil {
		jsonErr(w, "Servidor inacessível", 502)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		jsonErr(w, "Resposta inválida do servidor", 502)
		return
	}

	if ui, ok := result["user_info"]; ok {
		if uiMap, ok := ui.(map[string]interface{}); ok {
			if auth, ok := uiMap["auth"]; ok {
				if authF, ok := auth.(float64); ok && authF == 0 {
					jsonErr(w, "Usuário ou senha inválidos", 401)
					return
				}
			}
			if status, ok := uiMap["status"].(string); ok && status == "Disabled" {
				jsonErr(w, "Conta desativada", 403)
				return
			}
		}
	} else {
		jsonErr(w, "Falha na autenticação", 401)
		return
	}

	token := genToken()
	sessMu.Lock()
	sessions[token] = &Session{
		Username:  creds.Username,
		Password:  creds.Password,
		UserInfo:  body,
		CreatedAt: time.Now(),
	}
	sessMu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    token,
		Path:     "/",
		MaxAge:   86400 * 7,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie("session_token")
	if err == nil {
		sessMu.Lock()
		delete(sessions, c.Value)
		sessMu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: "session_token", Value: "", Path: "/", MaxAge: -1})
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

func handleSession(w http.ResponseWriter, r *http.Request) {
	sess := getSession(r)
	if sess == nil {
		jsonErr(w, "Not authenticated", 401)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(sess.UserInfo)
}

// Serve categories from local DB
func handleCategories(w http.ResponseWriter, r *http.Request) {
	sess := getSession(r)
	if sess == nil {
		jsonErr(w, "Not authenticated", 401)
		return
	}
	contentType := r.URL.Query().Get("type") // live, movie, series
	rows, err := db.Query("SELECT data_json FROM categories WHERE type = ? ORDER BY id", contentType)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
		return
	}
	defer rows.Close()

	var items []json.RawMessage
	for rows.Next() {
		var j string
		rows.Scan(&j)
		items = append(items, json.RawMessage(j))
	}
	if items == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

// Serve streams from local DB with pagination
func handleStreams(w http.ResponseWriter, r *http.Request) {
	sess := getSession(r)
	if sess == nil {
		jsonErr(w, "Not authenticated", 401)
		return
	}

	contentType := r.URL.Query().Get("type")
	catID := r.URL.Query().Get("category_id")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	search := r.URL.Query().Get("search")
	if page < 1 {
		page = 1
	}
	limit := cfg.ItemsPerPage
	offset := (page - 1) * limit

	var countQuery, dataQuery string
	var args []interface{}

	if search != "" {
		countQuery = "SELECT COUNT(*) FROM streams WHERE type = ? AND name LIKE ?"
		dataQuery = "SELECT data_json FROM streams WHERE type = ? AND name LIKE ? ORDER BY name LIMIT ? OFFSET ?"
		args = []interface{}{contentType, "%" + search + "%"}
	} else if catID != "" && catID != "all" {
		countQuery = "SELECT COUNT(*) FROM streams WHERE type = ? AND category_id = ?"
		dataQuery = "SELECT data_json FROM streams WHERE type = ? AND category_id = ? ORDER BY id LIMIT ? OFFSET ?"
		args = []interface{}{contentType, catID}
	} else {
		countQuery = "SELECT COUNT(*) FROM streams WHERE type = ?"
		dataQuery = "SELECT data_json FROM streams WHERE type = ? ORDER BY id LIMIT ? OFFSET ?"
		args = []interface{}{contentType}
	}

	var total int
	db.QueryRow(countQuery, args...).Scan(&total)

	dataArgs := append(args, limit, offset)
	rows, err := db.Query(dataQuery, dataArgs...)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[],"total":0,"page":1,"pages":1}`))
		return
	}
	defer rows.Close()

	var items []json.RawMessage
	for rows.Next() {
		var j string
		rows.Scan(&j)
		items = append(items, json.RawMessage(j))
	}
	if items == nil {
		items = []json.RawMessage{}
	}

	pages := total / limit
	if total%limit > 0 {
		pages++
	}
	if pages < 1 {
		pages = 1
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"items": items,
		"total": total,
		"page":  page,
		"pages": pages,
	})
}

// Search across all types
func handleSearch(w http.ResponseWriter, r *http.Request) {
	sess := getSession(r)
	if sess == nil {
		jsonErr(w, "Not authenticated", 401)
		return
	}

	q := r.URL.Query().Get("q")
	filterType := r.URL.Query().Get("type") // live, movie, series, or empty for all
	if q == "" {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
		return
	}

	var rows *sql.Rows
	var err error
	if filterType != "" {
		rows, err = db.Query("SELECT stream_id, name, type, category_id, icon, rating, data_json FROM streams WHERE type = ? AND name LIKE ? LIMIT 150", filterType, "%"+q+"%")
	} else {
		rows, err = db.Query("SELECT stream_id, name, type, category_id, icon, rating, data_json FROM streams WHERE name LIKE ? LIMIT 150", "%"+q+"%")
	}
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
		return
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var sid, name, stype, catID, icon, rating, dataJSON string
		rows.Scan(&sid, &name, &stype, &catID, &icon, &rating, &dataJSON)
		results = append(results, map[string]interface{}{
			"stream_id":   sid,
			"name":        name,
			"type":        stype,
			"category_id": catID,
			"icon":        icon,
			"rating":      rating,
		})
	}
	if results == nil {
		results = []map[string]interface{}{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// Proxy detail endpoints (vod_info, series_info, epg) — these are not cached
func handleProxy(w http.ResponseWriter, r *http.Request) {
	sess := getSession(r)
	if sess == nil {
		jsonErr(w, "Not authenticated", 401)
		return
	}

	action := r.URL.Query().Get("action")
	if action == "" {
		jsonErr(w, "Missing action", 400)
		return
	}

	apiURL := fmt.Sprintf("%s/player_api.php?username=%s&password=%s&action=%s",
		serverURL(), url.QueryEscape(sess.Username), url.QueryEscape(sess.Password), url.QueryEscape(action))

	for key, values := range r.URL.Query() {
		if key == "action" {
			continue
		}
		for _, v := range values {
			apiURL += "&" + url.QueryEscape(key) + "=" + url.QueryEscape(v)
		}
	}

	resp, err := httpClient.Get(apiURL)
	if err != nil {
		jsonErr(w, "Servidor inacessível", 502)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	io.Copy(w, resp.Body)
}

// Generate stream URL — direct to IPTV server
func handleStreamURL(w http.ResponseWriter, r *http.Request) {
	sess := getSession(r)
	if sess == nil {
		jsonErr(w, "Not authenticated", 401)
		return
	}

	stype := r.URL.Query().Get("type")
	sid := r.URL.Query().Get("id")
	ext := r.URL.Query().Get("ext")
	if ext == "" {
		if stype == "live" {
			ext = "m3u8"
		} else {
			ext = "mp4"
		}
	}

	streamURL := fmt.Sprintf("%s/%s/%s/%s/%s.%s",
		serverURL(), stype, sess.Username, sess.Password, sid, ext)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": streamURL})
}

// EPG
func handleEPG(w http.ResponseWriter, r *http.Request) {
	sess := getSession(r)
	if sess == nil {
		jsonErr(w, "Not authenticated", 401)
		return
	}

	streamID := r.URL.Query().Get("stream_id")
	apiURL := fmt.Sprintf("%s/player_api.php?username=%s&password=%s&action=get_short_epg&stream_id=%s&limit=20",
		serverURL(), url.QueryEscape(sess.Username), url.QueryEscape(sess.Password), url.QueryEscape(streamID))

	resp, err := httpClient.Get(apiURL)
	if err != nil {
		jsonErr(w, "Servidor inacessível", 502)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	io.Copy(w, resp.Body)
}

// ========== FAVORITES ==========

func handleFavorites(w http.ResponseWriter, r *http.Request) {
	sess := getSession(r)
	if sess == nil {
		jsonErr(w, "Not authenticated", 401)
		return
	}

	switch r.Method {
	case http.MethodGet:
		filterType := r.URL.Query().Get("type")
		var rows *sql.Rows
		var err error
		if filterType != "" && filterType != "all" {
			rows, err = db.Query("SELECT item_type, item_id, name, img FROM user_favorites WHERE username = ? AND item_type = ? ORDER BY created_at DESC", sess.Username, filterType)
		} else {
			rows, err = db.Query("SELECT item_type, item_id, name, img FROM user_favorites WHERE username = ? ORDER BY created_at DESC", sess.Username)
		}
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
			return
		}
		defer rows.Close()
		var items []map[string]string
		for rows.Next() {
			var itype, iid, name, img string
			rows.Scan(&itype, &iid, &name, &img)
			items = append(items, map[string]string{"type": itype, "id": iid, "name": name, "img": img})
		}
		if items == nil {
			items = []map[string]string{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(items)

	case http.MethodPost:
		var req struct {
			Action string `json:"action"` // add or remove
			Type   string `json:"type"`
			ID     string `json:"id"`
			Name   string `json:"name"`
			Img    string `json:"img"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		if req.Action == "remove" {
			db.Exec("DELETE FROM user_favorites WHERE username = ? AND item_type = ? AND item_id = ?", sess.Username, req.Type, req.ID)
		} else {
			db.Exec("INSERT OR REPLACE INTO user_favorites (username, item_type, item_id, name, img, created_at) VALUES (?, ?, ?, ?, ?, ?)",
				sess.Username, req.Type, req.ID, req.Name, req.Img, time.Now().Unix())
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	}
}

// Check if items are favorited (batch)
func handleFavCheck(w http.ResponseWriter, r *http.Request) {
	sess := getSession(r)
	if sess == nil {
		jsonErr(w, "Not authenticated", 401)
		return
	}
	rows, err := db.Query("SELECT item_type || '_' || item_id FROM user_favorites WHERE username = ?", sess.Username)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
		return
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var k string
		rows.Scan(&k)
		keys = append(keys, k)
	}
	if keys == nil {
		keys = []string{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(keys)
}

// ========== HISTORY ==========

func handleHistory(w http.ResponseWriter, r *http.Request) {
	sess := getSession(r)
	if sess == nil {
		jsonErr(w, "Not authenticated", 401)
		return
	}

	switch r.Method {
	case http.MethodGet:
		// For series: only show the latest episode per parent_id (series)
		// For live/movie: show all normally
		rows, err := db.Query(`
			SELECT item_type, item_id, COALESCE(parent_id,'') as parent_id, name, img, season, episode, position_sec, duration_sec, updated_at 
			FROM user_history WHERE username = ? 
			ORDER BY updated_at DESC LIMIT 100`, sess.Username)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
			return
		}
		defer rows.Close()

		var items []map[string]interface{}
		seenSeries := make(map[string]bool) // track parent_id to deduplicate series
		for rows.Next() {
			var itype, iid, parentID, name, img, season, episode string
			var pos, dur float64
			var updatedAt int64
			rows.Scan(&itype, &iid, &parentID, &name, &img, &season, &episode, &pos, &dur, &updatedAt)

			// Deduplicate series: only keep latest episode per series (parent_id)
			if itype == "series" && parentID != "" {
				if seenSeries[parentID] {
					continue
				}
				seenSeries[parentID] = true
			}

			items = append(items, map[string]interface{}{
				"type": itype, "id": iid, "parent_id": parentID,
				"name": name, "img": img,
				"season": season, "episode": episode,
				"position": pos, "duration": dur, "updated_at": updatedAt,
			})
		}
		if items == nil {
			items = []map[string]interface{}{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(items)

	case http.MethodPost:
		var req struct {
			Type     string  `json:"type"`
			ID       string  `json:"id"`
			ParentID string  `json:"parent_id"`
			Name     string  `json:"name"`
			Img      string  `json:"img"`
			Season   string  `json:"season"`
			Episode  string  `json:"episode"`
			Position float64 `json:"position"`
			Duration float64 `json:"duration"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		db.Exec(`INSERT OR REPLACE INTO user_history (username, item_type, item_id, parent_id, name, img, season, episode, position_sec, duration_sec, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			sess.Username, req.Type, req.ID, req.ParentID, req.Name, req.Img, req.Season, req.Episode, req.Position, req.Duration, time.Now().Unix())

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	}
}

// ========== CLEAR HISTORY ==========

func handleHistoryClear(w http.ResponseWriter, r *http.Request) {
	sess := getSession(r)
	if sess == nil {
		jsonErr(w, "Not authenticated", 401)
		return
	}
	if r.Method == http.MethodPost {
		filterType := r.URL.Query().Get("type")
		if filterType != "" {
			db.Exec("DELETE FROM user_history WHERE username = ? AND item_type = ?", sess.Username, filterType)
		} else {
			db.Exec("DELETE FROM user_history WHERE username = ?", sess.Username)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

func handleHistoryDelete(w http.ResponseWriter, r *http.Request) {
	sess := getSession(r)
	if sess == nil {
		jsonErr(w, "Not authenticated", 401)
		return
	}
	if r.Method == http.MethodPost {
		itemType := r.URL.Query().Get("type")
		itemID := r.URL.Query().Get("id")
		if itemType != "" && itemID != "" {
			db.Exec("DELETE FROM user_history WHERE username = ? AND item_type = ? AND item_id = ?", sess.Username, itemType, itemID)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

// ========== CONFIG ENDPOINT ==========

func handleConfig(w http.ResponseWriter, r *http.Request) {
	// Check if logo exists
	hasLogo := false
	logoURL := ""
	if cfg.LogoPath != "" {
		if _, err := os.Stat(cfg.LogoPath); err == nil {
			hasLogo = true
			logoURL = "/logo"
		}
	}
	// Also check default location
	if !hasLogo {
		if _, err := os.Stat("static/img/logo.png"); err == nil {
			hasLogo = true
			logoURL = "/logo"
		}
	}

	// Last sync info
	var lastSync string
	db.QueryRow("SELECT value FROM sync_meta WHERE key = 'last_sync'").Scan(&lastSync)

	var totalLive, totalMovies, totalSeries int
	db.QueryRow("SELECT COUNT(*) FROM streams WHERE type = 'live'").Scan(&totalLive)
	db.QueryRow("SELECT COUNT(*) FROM streams WHERE type = 'movie'").Scan(&totalMovies)
	db.QueryRow("SELECT COUNT(*) FROM streams WHERE type = 'series'").Scan(&totalSeries)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"player_name":   cfg.PlayerName,
		"has_logo":      hasLogo,
		"logo_url":      logoURL,
		"last_sync":     lastSync,
		"total_live":    totalLive,
		"total_movies":  totalMovies,
		"total_series":  totalSeries,
		"items_per_page": cfg.ItemsPerPage,
		"accent":         cfg.Accent,
		"accent_light":   cfg.AccentLight,
	})
}

func handleLogo(w http.ResponseWriter, r *http.Request) {
	path := cfg.LogoPath
	if path == "" {
		path = "static/img/logo.png"
	}
	if _, err := os.Stat(path); err != nil {
		http.NotFound(w, r)
		return
	}

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".jpg", ".jpeg":
		w.Header().Set("Content-Type", "image/jpeg")
	case ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
	case ".webp":
		w.Header().Set("Content-Type", "image/webp")
	default:
		w.Header().Set("Content-Type", "image/png")
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, path)
}

// ========== HELPERS ==========

func jsonErr(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func cleanSessions() {
	for {
		time.Sleep(1 * time.Hour)
		sessMu.Lock()
		for token, sess := range sessions {
			if time.Since(sess.CreatedAt) > 7*24*time.Hour {
				delete(sessions, token)
			}
		}
		sessMu.Unlock()
	}
}

// ========== PUBLIC BACKDROPS (no auth) ==========

func handlePublicBackdrops(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT icon FROM streams WHERE type = 'movie' AND icon != '' ORDER BY RANDOM() LIMIT 20")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
		return
	}
	defer rows.Close()
	var urls []string
	for rows.Next() {
		var icon string
		rows.Scan(&icon)
		if icon != "" {
			urls = append(urls, icon)
		}
	}
	if urls == nil {
		urls = []string{}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	json.NewEncoder(w).Encode(urls)
}

// ========== MAIN ==========

func main() {
	loadEnv()
	initDB()
	go cleanSessions()
	go startSyncLoop()

	// API
	http.HandleFunc("/api/login", handleLogin)
	http.HandleFunc("/api/public/backdrops", handlePublicBackdrops)
	http.HandleFunc("/api/logout", handleLogout)
	http.HandleFunc("/api/session", handleSession)
	http.HandleFunc("/api/config", handleConfig)
	http.HandleFunc("/api/categories", handleCategories)
	http.HandleFunc("/api/streams", handleStreams)
	http.HandleFunc("/api/search", handleSearch)
	http.HandleFunc("/api/proxy", handleProxy)
	http.HandleFunc("/api/stream-url", handleStreamURL)
	http.HandleFunc("/api/epg", handleEPG)
	http.HandleFunc("/api/favorites", handleFavorites)
	http.HandleFunc("/api/favorites/check", handleFavCheck)
	http.HandleFunc("/api/history", handleHistory)
	http.HandleFunc("/api/history/clear", handleHistoryClear)
	http.HandleFunc("/api/history/delete", handleHistoryDelete)
	http.HandleFunc("/logo", handleLogo)

	// Static
	http.Handle("/", http.FileServer(http.Dir("static")))

	port := cfg.Port
	if p := os.Getenv("PORT"); p != "" {
		port = p
	}

	fmt.Println("=====================================================")
	fmt.Println("📺 IPTV WebPlayer")
	fmt.Printf("   🌐 Servidor: %s\n", cfg.ServerURL)
	fmt.Printf("   🔄 Sync: %s / a cada %dh\n", cfg.SyncUser, cfg.SyncInterval)
	fmt.Printf("   📄 Itens/página: %d\n", cfg.ItemsPerPage)
	fmt.Printf("   🚀 Rodando em http://0.0.0.0:%s\n", port)
	fmt.Println("=====================================================")

	log.Fatal(http.ListenAndServe("0.0.0.0:"+port, nil))
}
