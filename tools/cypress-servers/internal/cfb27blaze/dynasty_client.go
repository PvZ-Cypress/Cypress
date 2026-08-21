package cfb27blaze

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type DynastySession struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	DynastyMode     string `json:"dynastyMode,omitempty"`
	CurrentWeek     int    `json:"currentWeek,omitempty"`
	Stage           string `json:"stage,omitempty"`
	MaxUsers        int    `json:"maxUsers,omitempty"`
	SavePath        string `json:"savePath,omitempty"`
	SaveSHA256      string `json:"saveSha256,omitempty"`
	SaveRevision    int    `json:"saveRevision,omitempty"`
	SelectedTeamKey int64  `json:"selectedTeamKey,omitempty"`
}

type DynastyClient struct {
	baseURL string
	client  *http.Client
}

func NewDynastyClient(baseURL string) *DynastyClient {
	return &DynastyClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *DynastyClient) EnsureSeeded(ctx context.Context, name string) (DynastySession, error) {
	sessions, err := c.ListSessions(ctx)
	if err != nil {
		return DynastySession{}, err
	}
	if len(sessions) > 0 {
		return sessions[0], nil
	}
	if strings.TrimSpace(name) == "" {
		name = "Local Dynasty"
	}
	return c.CreateSession(ctx, DynastySession{
		Name:        name,
		DynastyMode: "Online Dynasty",
		CurrentWeek: 1,
		Stage:       "preseason",
		MaxUsers:    32,
	})
}

func (c *DynastyClient) ListSessions(ctx context.Context) ([]DynastySession, error) {
	var response struct {
		Sessions []DynastySession `json:"sessions"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/sessions", nil, &response); err != nil {
		return nil, err
	}
	return response.Sessions, nil
}

func (c *DynastyClient) CreateSession(ctx context.Context, session DynastySession) (DynastySession, error) {
	var created DynastySession
	if err := c.doJSON(ctx, http.MethodPost, "/sessions", session, &created); err != nil {
		return DynastySession{}, err
	}
	return created, nil
}

func (c *DynastyClient) AdvanceSession(ctx context.Context, sessionID, userID int64, requestID string) (DynastySession, error) {
	var advanced DynastySession
	request := struct {
		UserID    int64  `json:"userId"`
		RequestID string `json:"requestId"`
	}{UserID: userID, RequestID: requestID}
	path := fmt.Sprintf("/sessions/%d/advance", sessionID)
	if err := c.doJSON(ctx, http.MethodPost, path, request, &advanced); err != nil {
		return DynastySession{}, err
	}
	return advanced, nil
}

func (c *DynastyClient) SelectTeam(ctx context.Context, sessionID, teamKey, coachKey int64) (DynastySession, error) {
	var selected DynastySession
	request := struct {
		TeamKey  int64 `json:"teamKey"`
		CoachKey int64 `json:"coachKey,omitempty"`
	}{TeamKey: teamKey, CoachKey: coachKey}
	path := fmt.Sprintf("/sessions/%d/select-team", sessionID)
	if err := c.doJSON(ctx, http.MethodPost, path, request, &selected); err != nil {
		return DynastySession{}, err
	}
	return selected, nil
}

// RecordJoin records a player joining a league in the Dynasty service's users
// table (the membership record the multi-user member list will read). It is
// best-effort from the caller's perspective: a join must still succeed to the
// client even if this bookkeeping call fails.
func (c *DynastyClient) RecordJoin(ctx context.Context, leagueID int64, displayName, teamID string) error {
	request := struct {
		DisplayName string `json:"displayName"`
		TeamID      string `json:"teamId"`
		IsAdmin     bool   `json:"isAdmin"`
	}{DisplayName: displayName, TeamID: teamID}
	path := fmt.Sprintf("/sessions/%d/users", leagueID)
	return c.doJSON(ctx, http.MethodPost, path, request, &struct{}{})
}

func (c *DynastyClient) doJSON(ctx context.Context, method, path string, requestBody any, responseBody any) error {
	var body io.Reader
	if requestBody != nil {
		encoded, err := json.Marshal(requestBody)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	if requestBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("dynasty service %s %s returned %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(message)))
	}
	return json.NewDecoder(resp.Body).Decode(responseBody)
}
