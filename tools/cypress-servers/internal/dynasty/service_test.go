package dynasty

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServiceCreatesSessionAndAction(t *testing.T) {
	root := filepath.Clean(`C:\Users\Shadow\Desktop\CFB27\Release\Dynasty_Assets\0`)
	svc, err := NewService(root, ":memory:")
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	defer svc.Close()

	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)

	session := postJSON(t, mux, "/sessions", map[string]any{
		"name":        "Saturday League",
		"stage":       "week_1",
		"currentWeek": 1,
		"maxUsers":    32,
	})
	if session.Code != http.StatusCreated {
		t.Fatalf("create session got %d: %s", session.Code, session.Body.String())
	}
	var created Session
	if err := json.Unmarshal(session.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	if created.ID == 0 || created.Name != "Saturday League" {
		t.Fatalf("bad session: %+v", created)
	}

	user := postJSON(t, mux, "/sessions/1/users", map[string]any{
		"displayName": "Shadow",
		"teamId":      "MICH",
		"isAdmin":     true,
	})
	if user.Code != http.StatusCreated {
		t.Fatalf("create user got %d: %s", user.Code, user.Body.String())
	}

	action := postJSON(t, mux, "/sessions/1/actions", map[string]any{
		"userId": 1,
		"type":   "AdvanceStageRequest",
		"payload": map[string]any{
			"targetStage": "week_2",
		},
	})
	if action.Code != http.StatusCreated {
		t.Fatalf("create action got %d: %s", action.Code, action.Body.String())
	}

	resolved := postJSON(t, mux, "/sessions/1/actions/1/resolve", map[string]any{
		"status": "resolved",
		"response": map[string]any{
			"ok": true,
		},
	})
	if resolved.Code != http.StatusOK {
		t.Fatalf("resolve action got %d: %s", resolved.Code, resolved.Body.String())
	}
	var resolvedAction Action
	if err := json.Unmarshal(resolved.Body.Bytes(), &resolvedAction); err != nil {
		t.Fatalf("decode action: %v", err)
	}
	if resolvedAction.Status != "resolved" || len(resolvedAction.Response) == 0 {
		t.Fatalf("bad resolved action: %+v", resolvedAction)
	}
}

func TestServiceSchemaEndpoints(t *testing.T) {
	root := filepath.Clean(`C:\Users\Shadow\Desktop\CFB27\Release\Dynasty_Assets\0`)
	svc, err := NewService(root, ":memory:")
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	defer svc.Close()

	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/schemas/uirequestforms", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("schema endpoint got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode schema response: %v", err)
	}
	// Game updates add UIRequestForms (486 introduced a 97th), so assert a floor
	// that still catches an empty or misconfigured schema root.
	if body.Count < 96 {
		t.Fatalf("expected at least 96 UIRequestForm schemas, got %d", body.Count)
	}
}

func TestAdvancePersistsOneLegalIdempotentTransition(t *testing.T) {
	root := filepath.Clean(`C:\Users\Shadow\Desktop\CFB27\Release\Dynasty_Assets\0`)
	dbFile := filepath.Join(t.TempDir(), "dynasty.db")
	svc, err := NewService(root, dbFile)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)

	if response := postJSON(t, mux, "/sessions", map[string]any{
		"name": "Persistent Dynasty", "stage": "preseason", "currentWeek": 1,
	}); response.Code != http.StatusCreated {
		t.Fatalf("create session: %d %s", response.Code, response.Body.String())
	}
	if response := postJSON(t, mux, "/sessions/1/users", map[string]any{
		"displayName": "Commissioner", "isAdmin": true,
	}); response.Code != http.StatusCreated {
		t.Fatalf("create commissioner: %d %s", response.Code, response.Body.String())
	}

	first := postJSON(t, mux, "/sessions/1/advance", map[string]any{"userId": 1, "requestId": "advance-1"})
	if first.Code != http.StatusOK {
		t.Fatalf("first advance: %d %s", first.Code, first.Body.String())
	}
	var afterFirst Session
	if err := json.Unmarshal(first.Body.Bytes(), &afterFirst); err != nil {
		t.Fatal(err)
	}
	if afterFirst.Stage != "week_1" || afterFirst.CurrentWeek != 1 {
		t.Fatalf("first transition = %+v, want preseason -> week_1", afterFirst)
	}

	repeated := postJSON(t, mux, "/sessions/1/advance", map[string]any{"userId": 1, "requestId": "advance-1"})
	var afterRepeat Session
	if repeated.Code != http.StatusOK || json.Unmarshal(repeated.Body.Bytes(), &afterRepeat) != nil {
		t.Fatalf("repeated advance: %d %s", repeated.Code, repeated.Body.String())
	}
	if afterRepeat.Stage != "week_1" || afterRepeat.CurrentWeek != 1 {
		t.Fatalf("duplicate request advanced twice: %+v", afterRepeat)
	}

	second := postJSON(t, mux, "/sessions/1/advance", map[string]any{"userId": 1, "requestId": "advance-2"})
	var afterSecond Session
	if second.Code != http.StatusOK || json.Unmarshal(second.Body.Bytes(), &afterSecond) != nil {
		t.Fatalf("second advance: %d %s", second.Code, second.Body.String())
	}
	if afterSecond.Stage != "week_2" || afterSecond.CurrentWeek != 2 {
		t.Fatalf("second transition = %+v, want week_2", afterSecond)
	}

	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewService(root, dbFile)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	persisted, err := reopened.getSession(1)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Stage != "week_2" || persisted.CurrentWeek != 2 {
		t.Fatalf("reopened session lost transition: %+v", persisted)
	}
}

func TestAdvanceRejectsNonCommissionerWithoutChangingState(t *testing.T) {
	root := filepath.Clean(`C:\Users\Shadow\Desktop\CFB27\Release\Dynasty_Assets\0`)
	svc, err := NewService(root, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)
	postJSON(t, mux, "/sessions", map[string]any{"name": "Protected", "stage": "preseason", "currentWeek": 1})
	postJSON(t, mux, "/sessions/1/users", map[string]any{"displayName": "Member", "isAdmin": false})

	response := postJSON(t, mux, "/sessions/1/advance", map[string]any{"userId": 1, "requestId": "forbidden"})
	if response.Code != http.StatusForbidden {
		t.Fatalf("non-commissioner advance = %d %s", response.Code, response.Body.String())
	}
	session, err := svc.getSession(1)
	if err != nil {
		t.Fatal(err)
	}
	if session.Stage != "preseason" || session.CurrentWeek != 1 {
		t.Fatalf("rejected transition changed state: %+v", session)
	}
}

func TestSessionOwnsPersistentFranchiseArtifact(t *testing.T) {
	root := filepath.Clean(`C:\Users\Shadow\Desktop\CFB27\Release\Dynasty_Assets\0`)
	seed := newestDynastySeed(t)
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is unavailable")
	}
	if seed == "" {
		t.Skip("attached full Dynasty save is unavailable")
	}
	tool, err := filepath.Abs(filepath.Join("..", "..", "cmd", "cfb27franchise", "main.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	cfg := Config{
		SchemaRoot:    root,
		DBFile:        filepath.Join(directory, "dynasty.db"),
		SeedFile:      seed,
		DataDir:       filepath.Join(directory, "artifacts"),
		NodePath:      node,
		FranchiseTool: tool,
	}
	sourceHash := testFileSHA256(t, seed)
	svc, err := NewServiceWithConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)

	createdResponse := postJSON(t, mux, "/sessions", map[string]any{
		"name": "Artifact Dynasty", "stage": "preseason", "currentWeek": 1,
	})
	if createdResponse.Code != http.StatusCreated {
		t.Fatalf("create session: %d %s", createdResponse.Code, createdResponse.Body.String())
	}
	var created Session
	if err := json.Unmarshal(createdResponse.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.SavePath == "" || created.SavePath == seed || created.SaveSHA256 == "" || created.SaveRevision != 0 {
		t.Fatalf("session has invalid artifact metadata: %+v", created)
	}
	if got := string(mustReadFile(t, created.SavePath)[:8]); got != "FBCHUNKS" {
		t.Fatalf("created artifact header = %q", got)
	}

	selectedResponse := postJSON(t, mux, "/sessions/1/select-team", map[string]any{"teamKey": 830865495})
	if selectedResponse.Code != http.StatusOK {
		t.Fatalf("select team: %d %s", selectedResponse.Code, selectedResponse.Body.String())
	}
	var selected Session
	if err := json.Unmarshal(selectedResponse.Body.Bytes(), &selected); err != nil {
		t.Fatal(err)
	}
	if selected.SelectedTeamKey != 830865495 || selected.SaveRevision != 1 || selected.SaveSHA256 == created.SaveSHA256 {
		t.Fatalf("team selection was not persisted: %+v", selected)
	}

	advancedResponse := postJSON(t, mux, "/sessions/1/advance", map[string]any{"requestId": "artifact-advance-1"})
	if advancedResponse.Code != http.StatusOK {
		t.Fatalf("advance: %d %s", advancedResponse.Code, advancedResponse.Body.String())
	}
	var advanced Session
	if err := json.Unmarshal(advancedResponse.Body.Bytes(), &advanced); err != nil {
		t.Fatal(err)
	}
	if advanced.Stage != "week_1" || advanced.CurrentWeek != 1 || advanced.SaveRevision != 2 || advanced.SaveSHA256 == selected.SaveSHA256 {
		t.Fatalf("advance was not persisted to the artifact: %+v", advanced)
	}
	if testFileSHA256(t, seed) != sourceHash {
		t.Fatal("session mutation overwrote the supplied offline save")
	}

	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewServiceWithConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	persisted, err := reopened.getSession(1)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.SavePath != advanced.SavePath || persisted.SaveSHA256 != advanced.SaveSHA256 || persisted.SaveRevision != 2 || persisted.SelectedTeamKey != 830865495 {
		t.Fatalf("reopened service lost artifact state: %+v", persisted)
	}
}

func TestExistingSQLiteOnlySessionIsBackfilledWithFranchiseArtifact(t *testing.T) {
	root := filepath.Clean(`C:\Users\Shadow\Desktop\CFB27\Release\Dynasty_Assets\0`)
	seed := newestDynastySeed(t)
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is unavailable")
	}
	if seed == "" {
		t.Skip("attached full Dynasty save is unavailable")
	}
	tool, err := filepath.Abs(filepath.Join("..", "..", "cmd", "cfb27franchise", "main.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	dbFile := filepath.Join(directory, "dynasty.db")
	legacy, err := NewService(root, dbFile)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	legacy.RegisterRoutes(mux)
	if response := postJSON(t, mux, "/sessions", map[string]any{
		"name": "Legacy Session", "stage": "week_6", "currentWeek": 6,
	}); response.Code != http.StatusCreated {
		t.Fatalf("create legacy session: %d %s", response.Code, response.Body.String())
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	upgraded, err := NewServiceWithConfig(Config{
		SchemaRoot: root, DBFile: dbFile, SeedFile: seed,
		DataDir: filepath.Join(directory, "artifacts"), NodePath: node, FranchiseTool: tool,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	session, err := upgraded.getSession(1)
	if err != nil {
		t.Fatal(err)
	}
	if session.SavePath == "" || session.SaveSHA256 == "" {
		t.Fatalf("legacy session was not backfilled: %+v", session)
	}
	if session.Stage != "preseason" || session.CurrentWeek != 1 || session.SaveRevision != 0 || session.SelectedTeamKey != 0 {
		t.Fatalf("legacy counters were not reconciled to the pristine seed artifact: %+v", session)
	}
	if got := string(mustReadFile(t, session.SavePath)[:8]); got != "FBCHUNKS" {
		t.Fatalf("backfilled artifact header = %q", got)
	}
}

// newestDynastySeed mirrors the launcher's Find-CFB27DynastySeed. The saves
// directory keeps Dynasty files written by older game builds, and a save only
// parses against the schema revision it was written with, so pinning one
// filename makes these tests fail after every game update rather than
// exercising the build that Dynasty_Assets currently holds.
func newestDynastySeed(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	directory := filepath.Join(home, "Documents", "EA SPORTS College Football 27", "Saves")
	entries, err := os.ReadDir(directory)
	if err != nil {
		return ""
	}
	newest := ""
	var newestTime time.Time
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(strings.ToUpper(entry.Name()), "DYNASTY") {
			continue
		}
		file := filepath.Join(directory, entry.Name())
		if err := validateFranchiseFile(file); err != nil {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if newest == "" || info.ModTime().After(newestTime) {
			newest, newestTime = file, info.ModTime()
		}
	}
	return newest
}

func testFileSHA256(t *testing.T, file string) string {
	t.Helper()
	data := mustReadFile(t, file)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func mustReadFile(t *testing.T, file string) []byte {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func postJSON(t *testing.T, h http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
