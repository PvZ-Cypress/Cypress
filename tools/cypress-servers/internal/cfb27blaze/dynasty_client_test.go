package cfb27blaze

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestDynastyClientEnsuresOneSeededSession(t *testing.T) {
	var mu sync.Mutex
	sessions := make([]DynastySession, 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/sessions":
			_ = json.NewEncoder(w).Encode(map[string]any{"sessions": sessions})
		case r.Method == http.MethodPost && r.URL.Path == "/sessions":
			var request DynastySession
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			request.ID = 1
			sessions = append(sessions, request)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(request)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewDynastyClient(server.URL)
	session, err := client.EnsureSeeded(t.Context(), "Local Dynasty")
	if err != nil {
		t.Fatal(err)
	}
	if session.ID != 1 || session.Name != "Local Dynasty" {
		t.Fatalf("unexpected seeded session: %#v", session)
	}

	again, err := client.EnsureSeeded(t.Context(), "Ignored Name")
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != session.ID {
		t.Fatalf("expected existing session, got %#v", again)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected exactly one session, got %d", len(sessions))
	}
}

func TestDynastyClientAdvancesWithIdempotencyKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/sessions/77/advance" {
			http.NotFound(w, r)
			return
		}
		var body struct {
			UserID    int64  `json:"userId"`
			RequestID string `json:"requestId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if body.UserID != 0 || body.RequestID != "connection-4:91" {
			t.Fatalf("unexpected advance request: %#v", body)
		}
		_ = json.NewEncoder(w).Encode(DynastySession{ID: 77, CurrentWeek: 2, Stage: "week_2"})
	}))
	defer server.Close()

	client := NewDynastyClient(server.URL)
	advanced, err := client.AdvanceSession(t.Context(), 77, 0, "connection-4:91")
	if err != nil {
		t.Fatal(err)
	}
	if advanced.CurrentWeek != 2 || advanced.Stage != "week_2" {
		t.Fatalf("unexpected advance response: %#v", advanced)
	}
}

func TestDynastyClientPersistsSelectedTeam(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/sessions/77/select-team" {
			http.NotFound(w, r)
			return
		}
		var body struct {
			TeamKey  int64 `json:"teamKey"`
			CoachKey int64 `json:"coachKey"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if body.TeamKey != 830865500 || body.CoachKey != 547356846 {
			t.Fatalf("unexpected team selection request: %#v", body)
		}
		_ = json.NewEncoder(w).Encode(DynastySession{ID: 77, SelectedTeamKey: body.TeamKey, SaveRevision: 1})
	}))
	defer server.Close()

	client := NewDynastyClient(server.URL)
	selected, err := client.SelectTeam(t.Context(), 77, 830865500, 547356846)
	if err != nil {
		t.Fatal(err)
	}
	if selected.SelectedTeamKey != 830865500 || selected.SaveRevision != 1 {
		t.Fatalf("unexpected selected-team response: %#v", selected)
	}
}
