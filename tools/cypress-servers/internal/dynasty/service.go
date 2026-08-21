package dynasty

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const serviceMaxBodySize = 1 << 20

const sessionColumns = `id, name, dynasty_mode, current_week, stage, max_users, password_hash != '', save_path, save_sha256, save_revision, selected_team_key, created_at, updated_at`

type Config struct {
	Bind          string
	Port          int
	SchemaRoot    string
	DBFile        string
	SeedFile      string
	DataDir       string
	NodePath      string
	FranchiseTool string
}

type Service struct {
	db         *sql.DB
	catalog    *Catalog
	store      *franchiseStore
	artifactMu sync.Mutex
}

type Session struct {
	ID              int64     `json:"id"`
	Name            string    `json:"name"`
	DynastyMode     string    `json:"dynastyMode"`
	CurrentWeek     int       `json:"currentWeek"`
	Stage           string    `json:"stage"`
	MaxUsers        int       `json:"maxUsers"`
	HasPassword     bool      `json:"hasPassword"`
	SavePath        string    `json:"savePath,omitempty"`
	SaveSHA256      string    `json:"saveSha256,omitempty"`
	SaveRevision    int       `json:"saveRevision"`
	SelectedTeamKey int64     `json:"selectedTeamKey,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type User struct {
	ID          int64     `json:"id"`
	SessionID   int64     `json:"sessionId"`
	DisplayName string    `json:"displayName"`
	TeamID      string    `json:"teamId,omitempty"`
	IsAdmin     bool      `json:"isAdmin"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Team struct {
	ID        int64     `json:"id"`
	SessionID int64     `json:"sessionId"`
	TeamID    string    `json:"teamId"`
	Name      string    `json:"name"`
	UserID    int64     `json:"userId,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Action struct {
	ID        int64           `json:"id"`
	SessionID int64           `json:"sessionId"`
	UserID    int64           `json:"userId,omitempty"`
	Type      string          `json:"type"`
	Status    string          `json:"status"`
	Payload   json.RawMessage `json:"payload"`
	Response  json.RawMessage `json:"response,omitempty"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

func Run(cfg Config) error {
	if cfg.Bind == "" {
		cfg.Bind = "0.0.0.0"
	}
	if cfg.Port == 0 {
		cfg.Port = 27910
	}
	if cfg.SchemaRoot == "" {
		cfg.SchemaRoot = `C:\Users\Shadow\Desktop\CFB27\Dynasty_Files`
	}
	if cfg.DBFile == "" {
		cfg.DBFile = "cfb27_dynasty.db"
	}

	svc, err := NewServiceWithConfig(cfg)
	if err != nil {
		return err
	}
	defer svc.Close()

	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)
	addr := fmt.Sprintf("%s:%d", cfg.Bind, cfg.Port)
	fmt.Printf("CFB27 Dynasty service listening on http://%s\n", addr)
	return http.ListenAndServe(addr, withCORS(mux))
}

func NewService(schemaRoot, dbFile string) (*Service, error) {
	return NewServiceWithConfig(Config{SchemaRoot: schemaRoot, DBFile: dbFile})
}

func NewServiceWithConfig(cfg Config) (*Service, error) {
	catalog, err := LoadCatalog(cfg.SchemaRoot)
	if err != nil {
		return nil, err
	}
	if err := ensureParentDir(cfg.DBFile); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", cfg.DBFile)
	if err != nil {
		return nil, err
	}
	store, err := newFranchiseStore(cfg)
	if err != nil {
		db.Close()
		return nil, err
	}
	svc := &Service{db: db, catalog: catalog, store: store}
	if err := svc.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	if err := svc.ensureSessionArtifacts(); err != nil {
		db.Close()
		return nil, err
	}
	return svc, nil
}

func (s *Service) ensureSessionArtifacts() error {
	if s.store == nil {
		return nil
	}
	rows, err := s.db.Query(`SELECT id, save_path FROM sessions ORDER BY id`)
	if err != nil {
		return err
	}
	type existingSession struct {
		id   int64
		path string
	}
	var sessions []existingSession
	for rows.Next() {
		var id int64
		var savePath string
		if err := rows.Scan(&id, &savePath); err != nil {
			rows.Close()
			return err
		}
		sessions = append(sessions, existingSession{id: id, path: savePath})
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, session := range sessions {
		if session.path != "" {
			if err := validateFranchiseFile(session.path); err != nil {
				return fmt.Errorf("session %d Dynasty artifact: %w", session.id, err)
			}
			continue
		}
		savePath, saveHash, err := s.store.create(context.Background(), session.id)
		if err != nil {
			return fmt.Errorf("backfill session %d Dynasty artifact: %w", session.id, err)
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := s.db.Exec(`UPDATE sessions SET save_path = ?, save_sha256 = ?, current_week = 1, stage = 'preseason', save_revision = 0, selected_team_key = 0, updated_at = ? WHERE id = ?`,
			savePath, saveHash, now, session.id); err != nil {
			_ = os.Remove(savePath)
			return err
		}
	}
	return nil
}

func (s *Service) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Service) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", method(s.handleHealth, nil))
	mux.HandleFunc("/schemas", method(s.handleSchemas, nil))
	mux.HandleFunc("/schemas/uirequestforms", method(s.handleUIRequestForms, nil))
	mux.HandleFunc("/sessions", method(s.handleSessionsGet, s.handleSessionsPost))
	mux.HandleFunc("/sessions/", s.handleSessionPath)
	mux.HandleFunc("/ws", method(s.handleWebSocketProbe, nil))
}

func (s *Service) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			dynasty_mode TEXT NOT NULL DEFAULT 'Online Dynasty',
			current_week INTEGER NOT NULL DEFAULT 1,
			stage TEXT NOT NULL DEFAULT 'preseason',
			max_users INTEGER NOT NULL DEFAULT 32,
			password_hash TEXT NOT NULL DEFAULT '',
			save_path TEXT NOT NULL DEFAULT '',
			save_sha256 TEXT NOT NULL DEFAULT '',
			save_revision INTEGER NOT NULL DEFAULT 0,
			selected_team_key INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id INTEGER NOT NULL,
			display_name TEXT NOT NULL,
			team_id TEXT NOT NULL DEFAULT '',
			is_admin INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS teams (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id INTEGER NOT NULL,
			team_id TEXT NOT NULL,
			name TEXT NOT NULL,
			user_id INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL,
			UNIQUE(session_id, team_id),
			FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS actions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id INTEGER NOT NULL,
			user_id INTEGER NOT NULL DEFAULT 0,
			type TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'queued',
			payload TEXT NOT NULL DEFAULT '{}',
			response TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS advance_requests (
			session_id INTEGER NOT NULL,
			request_id TEXT NOT NULL,
			result_week INTEGER NOT NULL,
			result_stage TEXT NOT NULL,
			created_at TEXT NOT NULL,
			PRIMARY KEY(session_id, request_id),
			FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	for name, definition := range map[string]string{
		"save_path":         "TEXT NOT NULL DEFAULT ''",
		"save_sha256":       "TEXT NOT NULL DEFAULT ''",
		"save_revision":     "INTEGER NOT NULL DEFAULT 0",
		"selected_team_key": "INTEGER NOT NULL DEFAULT 0",
	} {
		if err := ensureSQLiteColumn(s.db, "sessions", name, definition); err != nil {
			return err
		}
	}
	return nil
}

func ensureSQLiteColumn(db *sql.DB, table, column, definition string) error {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, dataType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		if name == column {
			found = true
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if found {
		return nil
	}
	_, err = db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + definition)
	return err
}

func (s *Service) handleHealth(w http.ResponseWriter, r *http.Request) {
	artifactConfigured := s.store != nil
	jsonResp(w, 200, map[string]any{
		"status":             "ok",
		"schemas":            s.catalog.SchemaCount,
		"files":              s.catalog.FileCount,
		"uiRequestFormCount": s.catalog.UIRequestFormCount,
		"artifactConfigured": artifactConfigured,
	})
}

func (s *Service) handleSchemas(w http.ResponseWriter, r *http.Request) {
	out := make([]Schema, 0, len(s.catalog.Schemas))
	for _, schema := range s.catalog.Schemas {
		out = append(out, schema)
	}
	jsonResp(w, 200, map[string]any{"schemas": out, "count": len(out)})
}

func (s *Service) handleUIRequestForms(w http.ResponseWriter, r *http.Request) {
	out := make([]Schema, 0, s.catalog.UIRequestFormCount)
	for _, schema := range s.catalog.Schemas {
		if schema.Base == "UIRequestForm" {
			out = append(out, schema)
		}
	}
	jsonResp(w, 200, map[string]any{"schemas": out, "count": len(out)})
}

func (s *Service) handleSessionsGet(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`SELECT ` + sessionColumns + ` FROM sessions ORDER BY updated_at DESC`)
	if err != nil {
		errResp(w, 500, err.Error())
		return
	}
	defer rows.Close()
	var sessions []Session
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			errResp(w, 500, err.Error())
			return
		}
		sessions = append(sessions, sess)
	}
	jsonResp(w, 200, map[string]any{"sessions": sessions})
}

func (s *Service) handleSessionsPost(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		DynastyMode string `json:"dynastyMode"`
		CurrentWeek int    `json:"currentWeek"`
		Stage       string `json:"stage"`
		MaxUsers    int    `json:"maxUsers"`
		Password    string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		errResp(w, 400, "invalid json")
		return
	}
	req.Name = trunc(strings.TrimSpace(req.Name), 128)
	if req.Name == "" {
		req.Name = "CFB27 Dynasty"
	}
	req.DynastyMode = defaultString(trunc(req.DynastyMode, 64), "Online Dynasty")
	req.Stage = defaultString(trunc(req.Stage, 64), "preseason")
	if req.CurrentWeek <= 0 {
		req.CurrentWeek = 1
	}
	req.MaxUsers = clamp(req.MaxUsers, 1, 256)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.Exec(`INSERT INTO sessions (name, dynasty_mode, current_week, stage, max_users, password_hash, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		req.Name, req.DynastyMode, req.CurrentWeek, req.Stage, req.MaxUsers, req.Password, now, now)
	if err != nil {
		errResp(w, 500, err.Error())
		return
	}
	id, _ := res.LastInsertId()
	if s.store != nil {
		savePath, saveHash, artifactErr := s.store.create(r.Context(), id)
		if artifactErr != nil {
			_, _ = s.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
			errResp(w, http.StatusInternalServerError, "create Dynasty artifact: "+artifactErr.Error())
			return
		}
		if _, err := s.db.Exec(`UPDATE sessions SET save_path = ?, save_sha256 = ? WHERE id = ?`, savePath, saveHash, id); err != nil {
			_ = os.Remove(savePath)
			_, _ = s.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
			errResp(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	sess, err := s.getSession(id)
	if err != nil {
		errResp(w, 500, err.Error())
		return
	}
	jsonResp(w, 201, sess)
}

func (s *Service) handleSessionPath(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/sessions/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		errResp(w, 404, "not found")
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		errResp(w, 400, "invalid session id")
		return
	}
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		s.handleSessionGet(w, id)
		return
	}
	switch parts[1] {
	case "users":
		if r.Method == http.MethodPost {
			s.handleUserPost(w, r, id)
			return
		}
	case "teams":
		if r.Method == http.MethodPost {
			s.handleTeamPost(w, r, id)
			return
		}
	case "select-team":
		if r.Method == http.MethodPost {
			s.handleSelectTeamPost(w, r, id)
			return
		}
	case "stage":
		if r.Method == http.MethodPost {
			s.handleStagePost(w, r, id)
			return
		}
	case "advance":
		if r.Method == http.MethodPost {
			s.handleAdvancePost(w, r, id)
			return
		}
	case "actions":
		if len(parts) == 2 && r.Method == http.MethodGet {
			s.handleActionsGet(w, id)
			return
		}
		if len(parts) == 2 && r.Method == http.MethodPost {
			s.handleActionPost(w, r, id)
			return
		}
		if len(parts) == 4 && parts[3] == "resolve" && r.Method == http.MethodPost {
			actionID, err := strconv.ParseInt(parts[2], 10, 64)
			if err != nil {
				errResp(w, 400, "invalid action id")
				return
			}
			s.handleActionResolve(w, r, id, actionID)
			return
		}
	}
	methodNotAllowed(w)
}

func (s *Service) handleSessionGet(w http.ResponseWriter, id int64) {
	sess, err := s.getSession(id)
	if err != nil {
		errResp(w, 404, "session not found")
		return
	}
	users, _ := s.getUsers(id)
	teams, _ := s.getTeams(id)
	actions, _ := s.getActions(id)
	jsonResp(w, 200, map[string]any{"session": sess, "users": users, "teams": teams, "actions": actions})
}

func (s *Service) handleUserPost(w http.ResponseWriter, r *http.Request, sessionID int64) {
	var req struct {
		DisplayName string `json:"displayName"`
		TeamID      string `json:"teamId"`
		IsAdmin     bool   `json:"isAdmin"`
	}
	if err := readJSON(r, &req); err != nil {
		errResp(w, 400, "invalid json")
		return
	}
	req.DisplayName = trunc(strings.TrimSpace(req.DisplayName), 64)
	if req.DisplayName == "" {
		errResp(w, 400, "displayName required")
		return
	}
	req.TeamID = trunc(strings.TrimSpace(req.TeamID), 32)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.Exec(`INSERT INTO users (session_id, display_name, team_id, is_admin, created_at) VALUES (?, ?, ?, ?, ?)`,
		sessionID, req.DisplayName, req.TeamID, boolInt(req.IsAdmin), now)
	if err != nil {
		errResp(w, 500, err.Error())
		return
	}
	id, _ := res.LastInsertId()
	jsonResp(w, 201, map[string]any{"id": id, "sessionId": sessionID, "displayName": req.DisplayName, "teamId": req.TeamID, "isAdmin": req.IsAdmin, "createdAt": now})
}

func (s *Service) handleTeamPost(w http.ResponseWriter, r *http.Request, sessionID int64) {
	var req struct {
		TeamID string `json:"teamId"`
		Name   string `json:"name"`
		UserID int64  `json:"userId"`
	}
	if err := readJSON(r, &req); err != nil {
		errResp(w, 400, "invalid json")
		return
	}
	req.TeamID = trunc(strings.TrimSpace(req.TeamID), 32)
	req.Name = trunc(strings.TrimSpace(req.Name), 128)
	if req.TeamID == "" || req.Name == "" {
		errResp(w, 400, "teamId and name required")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.Exec(`INSERT INTO teams (session_id, team_id, name, user_id, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(session_id, team_id) DO UPDATE SET name = excluded.name, user_id = excluded.user_id, updated_at = excluded.updated_at`,
		sessionID, req.TeamID, req.Name, req.UserID, now)
	if err != nil {
		errResp(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, map[string]any{"sessionId": sessionID, "teamId": req.TeamID, "name": req.Name, "userId": req.UserID, "updatedAt": now})
}

func (s *Service) handleSelectTeamPost(w http.ResponseWriter, r *http.Request, sessionID int64) {
	var req struct {
		TeamKey  int64 `json:"teamKey"`
		CoachKey int64 `json:"coachKey,omitempty"`
	}
	if err := readJSON(r, &req); err != nil {
		errResp(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.TeamKey <= 0 {
		errResp(w, http.StatusBadRequest, "teamKey required")
		return
	}
	if s.store == nil {
		errResp(w, http.StatusServiceUnavailable, "Dynasty franchise persistence is not configured")
		return
	}

	s.artifactMu.Lock()
	defer s.artifactMu.Unlock()
	session, err := s.getSession(sessionID)
	if err != nil {
		errResp(w, http.StatusNotFound, "session not found")
		return
	}
	if session.SavePath == "" {
		errResp(w, http.StatusConflict, "session has no Dynasty artifact")
		return
	}
	mutation, err := s.store.selectTeam(r.Context(), session.SavePath, req.TeamKey, req.CoachKey)
	if err != nil {
		errResp(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	defer os.Remove(mutation.temporaryPath)
	commitArtifact, rollbackArtifact, err := swapFranchiseArtifact(session.SavePath, mutation)
	if err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.db.Exec(`UPDATE sessions SET selected_team_key = ?, save_sha256 = ?, save_revision = save_revision + 1, updated_at = ? WHERE id = ?`,
		req.TeamKey, mutation.sha256, now, sessionID); err != nil {
		rollbackArtifact()
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	commitArtifact()
	updated, err := s.getSession(sessionID)
	if err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResp(w, http.StatusOK, updated)
}

func (s *Service) handleStagePost(w http.ResponseWriter, r *http.Request, sessionID int64) {
	var req struct {
		CurrentWeek int    `json:"currentWeek"`
		Stage       string `json:"stage"`
	}
	if err := readJSON(r, &req); err != nil {
		errResp(w, 400, "invalid json")
		return
	}
	req.Stage = trunc(strings.TrimSpace(req.Stage), 64)
	if req.Stage == "" {
		errResp(w, 400, "stage required")
		return
	}
	if req.CurrentWeek <= 0 {
		req.CurrentWeek = 1
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.Exec(`UPDATE sessions SET current_week = ?, stage = ?, updated_at = ? WHERE id = ?`, req.CurrentWeek, req.Stage, now, sessionID)
	if err != nil {
		errResp(w, 500, err.Error())
		return
	}
	sess, err := s.getSession(sessionID)
	if err != nil {
		errResp(w, 404, "session not found")
		return
	}
	jsonResp(w, 200, sess)
}

func (s *Service) handleAdvancePost(w http.ResponseWriter, r *http.Request, sessionID int64) {
	var req struct {
		UserID    int64  `json:"userId"`
		RequestID string `json:"requestId"`
	}
	if err := readJSON(r, &req); err != nil {
		errResp(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.RequestID = trunc(strings.TrimSpace(req.RequestID), 128)
	if req.RequestID == "" {
		errResp(w, http.StatusBadRequest, "requestId required")
		return
	}
	s.artifactMu.Lock()
	defer s.artifactMu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	if req.UserID != 0 {
		var isAdmin int
		if err := tx.QueryRow(`SELECT is_admin FROM users WHERE session_id = ? AND id = ?`, sessionID, req.UserID).Scan(&isAdmin); err != nil || isAdmin == 0 {
			errResp(w, http.StatusForbidden, "commissioner required")
			return
		}
	}

	var existingWeek int
	var existingStage string
	if err := tx.QueryRow(`SELECT result_week, result_stage FROM advance_requests WHERE session_id = ? AND request_id = ?`, sessionID, req.RequestID).Scan(&existingWeek, &existingStage); err == nil {
		session, err := sessionFromTx(tx, sessionID)
		if err != nil {
			errResp(w, http.StatusNotFound, "session not found")
			return
		}
		session.CurrentWeek = existingWeek
		session.Stage = existingStage
		jsonResp(w, http.StatusOK, session)
		return
	}

	session, err := sessionFromTx(tx, sessionID)
	if err != nil {
		errResp(w, http.StatusNotFound, "session not found")
		return
	}
	nextWeek, nextStage, ok := nextDynastyStage(session.CurrentWeek, session.Stage)
	if !ok {
		errResp(w, http.StatusConflict, "illegal dynasty stage transition")
		return
	}
	var commitArtifact, rollbackArtifact func()
	var mutation franchiseMutation
	if session.SavePath != "" {
		if s.store == nil {
			errResp(w, http.StatusServiceUnavailable, "Dynasty franchise persistence is not configured")
			return
		}
		mutation, err = s.store.advance(r.Context(), session.SavePath, nextWeek, nextStage)
		if err != nil {
			errResp(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		defer os.Remove(mutation.temporaryPath)
		commitArtifact, rollbackArtifact, err = swapFranchiseArtifact(session.SavePath, mutation)
		if err != nil {
			errResp(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	updateQuery := `UPDATE sessions SET current_week = ?, stage = ?, updated_at = ? WHERE id = ?`
	updateArgs := []any{nextWeek, nextStage, now, sessionID}
	if commitArtifact != nil {
		updateQuery = `UPDATE sessions SET current_week = ?, stage = ?, save_sha256 = ?, save_revision = save_revision + 1, updated_at = ? WHERE id = ?`
		updateArgs = []any{nextWeek, nextStage, mutation.sha256, now, sessionID}
	}
	if _, err := tx.Exec(updateQuery, updateArgs...); err != nil {
		if rollbackArtifact != nil {
			rollbackArtifact()
		}
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := tx.Exec(`INSERT INTO advance_requests (session_id, request_id, result_week, result_stage, created_at) VALUES (?, ?, ?, ?, ?)`, sessionID, req.RequestID, nextWeek, nextStage, now); err != nil {
		if rollbackArtifact != nil {
			rollbackArtifact()
		}
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		if rollbackArtifact != nil {
			rollbackArtifact()
		}
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	if commitArtifact != nil {
		commitArtifact()
		session.SaveSHA256 = mutation.sha256
		session.SaveRevision++
	}
	session.CurrentWeek = nextWeek
	session.Stage = nextStage
	session.UpdatedAt = parseTime(now)
	jsonResp(w, http.StatusOK, session)
}

func sessionFromTx(tx *sql.Tx, id int64) (Session, error) {
	row := tx.QueryRow(`SELECT `+sessionColumns+` FROM sessions WHERE id = ?`, id)
	return scanSession(row)
}

func nextDynastyStage(currentWeek int, stage string) (int, string, bool) {
	stage = strings.ToLower(strings.TrimSpace(stage))
	if stage == "preseason" {
		return 1, "week_1", true
	}
	if !strings.HasPrefix(stage, "week_") {
		return 0, "", false
	}
	week, err := strconv.Atoi(strings.TrimPrefix(stage, "week_"))
	if err != nil || week < 1 || week != currentWeek {
		return 0, "", false
	}
	week++
	return week, fmt.Sprintf("week_%d", week), true
}

func (s *Service) handleActionsGet(w http.ResponseWriter, sessionID int64) {
	actions, err := s.getActions(sessionID)
	if err != nil {
		errResp(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, map[string]any{"actions": actions})
}

func (s *Service) handleActionPost(w http.ResponseWriter, r *http.Request, sessionID int64) {
	var req struct {
		UserID  int64           `json:"userId"`
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := readJSON(r, &req); err != nil {
		errResp(w, 400, "invalid json")
		return
	}
	req.Type = trunc(strings.TrimSpace(req.Type), 128)
	if req.Type == "" {
		errResp(w, 400, "type required")
		return
	}
	if len(req.Payload) == 0 {
		req.Payload = json.RawMessage(`{}`)
	}
	if !json.Valid(req.Payload) {
		errResp(w, 400, "payload must be json")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.Exec(`INSERT INTO actions (session_id, user_id, type, status, payload, created_at, updated_at) VALUES (?, ?, ?, 'queued', ?, ?, ?)`,
		sessionID, req.UserID, req.Type, string(req.Payload), now, now)
	if err != nil {
		errResp(w, 500, err.Error())
		return
	}
	id, _ := res.LastInsertId()
	action, err := s.getAction(sessionID, id)
	if err != nil {
		errResp(w, 500, err.Error())
		return
	}
	jsonResp(w, 201, action)
}

func (s *Service) handleActionResolve(w http.ResponseWriter, r *http.Request, sessionID, actionID int64) {
	var req struct {
		Status   string          `json:"status"`
		Response json.RawMessage `json:"response"`
	}
	if err := readJSON(r, &req); err != nil {
		errResp(w, 400, "invalid json")
		return
	}
	req.Status = trunc(strings.TrimSpace(req.Status), 32)
	if req.Status == "" {
		req.Status = "resolved"
	}
	if len(req.Response) == 0 {
		req.Response = json.RawMessage(`{}`)
	}
	if !json.Valid(req.Response) {
		errResp(w, 400, "response must be json")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.Exec(`UPDATE actions SET status = ?, response = ?, updated_at = ? WHERE session_id = ? AND id = ?`,
		req.Status, string(req.Response), now, sessionID, actionID)
	if err != nil {
		errResp(w, 500, err.Error())
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		errResp(w, 404, "action not found")
		return
	}
	action, err := s.getAction(sessionID, actionID)
	if err != nil {
		errResp(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, action)
}

func (s *Service) handleWebSocketProbe(w http.ResponseWriter, r *http.Request) {
	if strings.ToLower(r.Header.Get("Upgrade")) == "websocket" {
		errResp(w, 501, "websocket event stream is reserved for the runtime bridge; use HTTP polling in v1")
		return
	}
	jsonResp(w, 200, map[string]string{"status": "reserved", "message": "HTTP APIs are active; WebSocket bridge will attach after CFB27 runtime endpoints are mapped"})
}

func (s *Service) getSession(id int64) (Session, error) {
	row := s.db.QueryRow(`SELECT `+sessionColumns+` FROM sessions WHERE id = ?`, id)
	return scanSession(row)
}

func (s *Service) getUsers(sessionID int64) ([]User, error) {
	rows, err := s.db.Query(`SELECT id, session_id, display_name, team_id, is_admin, created_at FROM users WHERE session_id = ? ORDER BY id`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		var u User
		var isAdmin int
		var created string
		if err := rows.Scan(&u.ID, &u.SessionID, &u.DisplayName, &u.TeamID, &isAdmin, &created); err != nil {
			return nil, err
		}
		u.IsAdmin = isAdmin != 0
		u.CreatedAt = parseTime(created)
		users = append(users, u)
	}
	return users, nil
}

func (s *Service) getTeams(sessionID int64) ([]Team, error) {
	rows, err := s.db.Query(`SELECT id, session_id, team_id, name, user_id, updated_at FROM teams WHERE session_id = ? ORDER BY team_id`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var teams []Team
	for rows.Next() {
		var t Team
		var updated string
		if err := rows.Scan(&t.ID, &t.SessionID, &t.TeamID, &t.Name, &t.UserID, &updated); err != nil {
			return nil, err
		}
		t.UpdatedAt = parseTime(updated)
		teams = append(teams, t)
	}
	return teams, nil
}

func (s *Service) getActions(sessionID int64) ([]Action, error) {
	rows, err := s.db.Query(`SELECT id, session_id, user_id, type, status, payload, response, created_at, updated_at FROM actions WHERE session_id = ? ORDER BY id`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var actions []Action
	for rows.Next() {
		action, err := scanAction(rows)
		if err != nil {
			return nil, err
		}
		actions = append(actions, action)
	}
	return actions, nil
}

func (s *Service) getAction(sessionID, actionID int64) (Action, error) {
	row := s.db.QueryRow(`SELECT id, session_id, user_id, type, status, payload, response, created_at, updated_at FROM actions WHERE session_id = ? AND id = ?`, sessionID, actionID)
	return scanAction(row)
}

type scanner interface {
	Scan(dest ...any) error
}

func scanSession(row scanner) (Session, error) {
	var sess Session
	var hasPassword bool
	var created, updated string
	if err := row.Scan(
		&sess.ID, &sess.Name, &sess.DynastyMode, &sess.CurrentWeek, &sess.Stage,
		&sess.MaxUsers, &hasPassword, &sess.SavePath, &sess.SaveSHA256,
		&sess.SaveRevision, &sess.SelectedTeamKey, &created, &updated,
	); err != nil {
		return Session{}, err
	}
	sess.HasPassword = hasPassword
	sess.CreatedAt = parseTime(created)
	sess.UpdatedAt = parseTime(updated)
	return sess, nil
}

func scanAction(row scanner) (Action, error) {
	var action Action
	var payload, response, created, updated string
	if err := row.Scan(&action.ID, &action.SessionID, &action.UserID, &action.Type, &action.Status, &payload, &response, &created, &updated); err != nil {
		return Action{}, err
	}
	action.Payload = json.RawMessage(defaultString(payload, "{}"))
	if response != "" {
		action.Response = json.RawMessage(response)
	}
	action.CreatedAt = parseTime(created)
	action.UpdatedAt = parseTime(updated)
	return action, nil
}

func readJSON(r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, serviceMaxBodySize)
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(dst)
}

func jsonResp(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func errResp(w http.ResponseWriter, code int, msg string) {
	jsonResp(w, code, map[string]any{"ok": false, "error": msg})
}

func method(get, post http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == http.MethodGet && get != nil {
			get(w, r)
			return
		}
		if r.Method == http.MethodPost && post != nil {
			post(w, r)
			return
		}
		methodNotAllowed(w)
	}
}

func methodNotAllowed(w http.ResponseWriter) {
	errResp(w, http.StatusMethodNotAllowed, "method not allowed")
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		next.ServeHTTP(w, r)
	})
}

func ensureParentDir(path string) error {
	if path == "" || path == ":memory:" {
		return nil
	}
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0755)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func trunc(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func defaultString(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func parseTime(v string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return time.Time{}
	}
	return t
}
