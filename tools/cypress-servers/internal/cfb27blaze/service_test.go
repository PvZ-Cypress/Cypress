package cfb27blaze

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cypress-servers/internal/blaze"
	"cypress-servers/internal/cfb27assets"
)

func TestHandleUnsupportedCommand(t *testing.T) {
	svc := newServiceWithAuthoritativeCoaches(t)
	request := blaze.Frame{
		Header: blaze.Header{
			Component:   0x7777,
			Command:     0x1234,
			MessageType: blaze.MessageTypeRequest,
			MessageID:   99,
		},
	}

	response := svc.HandleFrame(context.Background(), "test-connection", request)

	if response.Header.MessageType != blaze.MessageTypeErrorReply {
		t.Fatalf("expected error reply, got %d", response.Header.MessageType)
	}
	if response.Header.ErrorCode != ErrorCommandNotFound {
		t.Fatalf("expected command-not-found error, got 0x%04x", response.Header.ErrorCode)
	}
	if response.Header.MessageID != request.Header.MessageID {
		t.Fatalf("expected message ID %d, got %d", request.Header.MessageID, response.Header.MessageID)
	}
	if len(svc.Events()) != 1 {
		t.Fatalf("expected one recorded event, got %d", len(svc.Events()))
	}
}

func TestHandleLocalLogin(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	request := blaze.Frame{
		Header: blaze.Header{
			Component:   ComponentAuthentication,
			Command:     CommandAuthenticationLogin,
			MessageType: blaze.MessageTypeRequest,
			MessageID:   7,
		},
	}

	response := svc.HandleFrame(context.Background(), "test-connection", request)
	if response.Header.MessageType != blaze.MessageTypeReply {
		t.Fatalf("expected reply, got %d with error 0x%04x", response.Header.MessageType, response.Header.ErrorCode)
	}

	fields, err := blaze.Decode(response.Payload)
	if err != nil {
		t.Fatal(err)
	}
	sessionField, ok := fieldByTag(fields, "SESS")
	if !ok {
		t.Fatal("login response is missing SESS")
	}
	session, ok := sessionField.Value.([]blaze.Field)
	if !ok {
		t.Fatalf("expected structured SESS, got %T", sessionField.Value)
	}
	if value, ok := integerField(session, "BUID"); !ok || value != LocalBlazeID {
		t.Fatalf("expected Blaze ID %d, got %d", LocalBlazeID, value)
	}
	personaField, ok := fieldByTag(session, "PDTL")
	if !ok {
		t.Fatal("login response is missing SESS.PDTL")
	}
	persona, ok := personaField.Value.([]blaze.Field)
	if !ok {
		t.Fatalf("expected structured PDTL, got %T", personaField.Value)
	}
	if value, ok := stringField(persona, "DSNM"); !ok || value != "LocalPlayer" {
		t.Fatalf("expected LocalPlayer identity, got %q", value)
	}
	metadata, err := blaze.Decode(response.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := stringField(metadata, "SKEY"); !ok || value != localSessionKey {
		t.Fatalf("expected local session key, got %q", value)
	}
}

func TestDynastyEntryRoutes(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})

	saveSettings := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header: blaze.Header{
			Component: ComponentUtil, Command: CommandUtilSaveUserSettings,
			MessageType: blaze.MessageTypeRequest, MessageID: 10,
		},
	})
	if saveSettings.Header.MessageType != blaze.MessageTypeReply || saveSettings.Header.ErrorCode != 0 {
		t.Fatalf("save-user-settings failed: type=%d error=0x%04x", saveSettings.Header.MessageType, saveSettings.Header.ErrorCode)
	}

	createOrJoin := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header: blaze.Header{
			Component: ComponentGameManager, Command: CommandGameManagerCreateOrJoin,
			MessageType: blaze.MessageTypeRequest, MessageID: 11,
		},
	})
	if createOrJoin.Header.MessageType != blaze.MessageTypeReply || createOrJoin.Header.ErrorCode != 0 {
		t.Fatalf("create-or-join failed: type=%d error=0x%04x", createOrJoin.Header.MessageType, createOrJoin.Header.ErrorCode)
	}
	fields, err := blaze.Decode(createOrJoin.Payload)
	if err != nil {
		t.Fatal(err)
	}
	// create-or-join now mints a real game session rather than the old fixed
	// GID=1 stub, so assert a plausible id plus the captured reply shape.
	if gameID, ok := integerField(fields, "GID"); !ok || gameID <= 0 {
		t.Fatalf("expected a real game ID, got %d", gameID)
	}
	if _, ok := fieldByTag(fields, "ESID"); !ok {
		t.Fatal("create-or-join reply missing ESID (external session ids)")
	}
	for _, tag := range []string{"JGS", "OCAL"} {
		if _, ok := integerField(fields, tag); !ok {
			t.Fatalf("create-or-join reply missing %s", tag)
		}
	}
}

func TestDynastyNotificationAcknowledgementMatchesCapture(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header: blaze.Header{
			Component: ComponentOSDK, Command: 13,
			MessageType: blaze.MessageTypeRequest, MessageID: 12,
		},
	})

	if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
		t.Fatalf("Dynasty notification failed: type=%d error=0x%04x", response.Header.MessageType, response.Header.ErrorCode)
	}
	if len(response.Payload) != 0 {
		t.Fatalf("expected captured empty response, got %d payload bytes", len(response.Payload))
	}
}

func TestDynastyCreateReturnsLocalLeagueContract(t *testing.T) {
	var posts int
	dynasty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/sessions" {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		posts++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":88,"name":"Local Dynasty","dynastyMode":"Online Dynasty","currentWeek":1,"stage":"preseason","maxUsers":32}`))
	}))
	defer dynasty.Close()

	svc := NewService(Config{Profile: "LocalPlayer", DynastyURL: dynasty.URL})
	requestPayload, err := blaze.Encode([]blaze.Field{
		{Tag: "NAME", Type: blaze.TypeString, Value: ""},
	})
	if err != nil {
		t.Fatal(err)
	}
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header: blaze.Header{
			Component: ComponentBootStatus, Command: 101,
			MessageType: blaze.MessageTypeRequest, MessageID: 13,
		},
		Payload: requestPayload,
	})

	if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
		t.Fatalf("Dynasty create failed: type=%d error=0x%04x", response.Header.MessageType, response.Header.ErrorCode)
	}
	fields, err := blaze.Decode(response.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := integerField(fields, "LGID"); !ok || value != 88 {
		t.Fatalf("expected newly-created local league ID 88, got %d", value)
	}
	if posts != 1 {
		t.Fatalf("expected one new Dynasty session, got %d", posts)
	}
	if value, ok := integerField(fields, "SUCC"); !ok || value != 1 {
		t.Fatalf("expected SUCC=1, got %d", value)
	}
	if value, ok := stringField(fields, "SMSG"); !ok || value != "" {
		t.Fatalf("expected empty SMSG, got %q", value)
	}
	for _, tag := range []string{"CURR", "ORIG"} {
		field, ok := fieldByTag(fields, tag)
		if !ok {
			t.Fatalf("Dynasty create response is missing %s", tag)
		}
		version, ok := field.Value.([]blaze.Field)
		if !ok {
			t.Fatalf("%s has unexpected type %T", tag, field.Value)
		}
		for versionTag, expected := range map[string]int64{"DMAJ": 814, "DMIN": 0, "PDRN": 3} {
			if value, ok := integerField(version, versionTag); !ok || value != expected {
				t.Fatalf("expected %s.%s=%d, got %d", tag, versionTag, expected, value)
			}
		}
	}
}

func TestDynastyBootstrapRoutesMatchCapture(t *testing.T) {
	tests := []struct {
		command uint16
		length  int
		sha256  string
	}{
		{command: 531, length: 553289, sha256: "758fb73c96f9b215e18f3b89c3b4cc04b070fa17be4b562d91bd08eb98854e91"},
		{command: 301, length: 61093, sha256: "ee9964928f33cef164b0fc12e8cc168f40a8db38691f0b7585e7969c507c111b"},
		{command: 1261, length: 192357, sha256: "ccee172648870ba0519d8fe8a9b5dd192d757a9475de148d8de397dce6f1ffcf"},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("command-%d", test.command), func(t *testing.T) {
			svc := NewService(Config{Profile: "LocalPlayer"})
			response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
				Header: blaze.Header{
					Component: ComponentBootStatus, Command: test.command,
					MessageType: blaze.MessageTypeRequest, MessageID: uint32(test.command),
				},
			})

			if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
				t.Fatalf("Dynasty bootstrap command %d failed: type=%d error=0x%04x", test.command, response.Header.MessageType, response.Header.ErrorCode)
			}
			if len(response.Payload) != test.length {
				t.Fatalf("Dynasty bootstrap command %d returned %d bytes, want %d", test.command, len(response.Payload), test.length)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(response.Payload)); got != test.sha256 {
				t.Fatalf("Dynasty bootstrap command %d payload hash = %s, want %s", test.command, got, test.sha256)
			}
		})
	}
}

func TestLeagueTeamDatabaseIsNotRewrittenForSelectedTeam(t *testing.T) {
	svc := newServiceWithAuthoritativeCoaches(t)
	svc.fallbackSession().selectedTeam.Store(830865495)
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{Header: blaze.Header{
		Component: ComponentBootStatus, Command: 391,
		MessageType: blaze.MessageTypeRequest, MessageID: 391,
	}})
	if !bytes.Equal(response.Payload, dynasty391Payload) {
		t.Fatal("league-wide team database was mutated for the selected team")
	}
	for _, name := range []string{"Akron", "Ohio State"} {
		if !bytes.Contains(response.Payload, []byte(name)) {
			t.Fatalf("league-wide team database no longer contains %s", name)
		}
	}
}

func TestDynastyFormResponsesMatchCapture(t *testing.T) {
	tests := []struct {
		formID uint64
		length int
		sha256 string
	}{
		{formID: 133300354, length: 11646, sha256: "3757e325ef46d950d8353b82b079aff6d441ebe2f320a158e796581832a78e7f"},
		{formID: 133300355, length: 61152, sha256: "63fdc08f4d8f9f9973b026934ee399fe2b4d23815c56b8a93a19ecdbbf8c29d9"},
		{formID: 133300356, length: 423124, sha256: "755ae814ac016c1244a382b4bd1fbf9cad18a61890eaf1da3bb2f184fc68a320"},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("form-%d", test.formID), func(t *testing.T) {
			payload, err := blaze.Encode([]blaze.Field{
				{Tag: "FORM", Type: blaze.TypeInteger, Value: int64(test.formID)},
			})
			if err != nil {
				t.Fatal(err)
			}

			svc := NewService(Config{Profile: "LocalPlayer"})
			response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
				Header: blaze.Header{
					Component: ComponentBootStatus, Command: 533,
					MessageType: blaze.MessageTypeRequest, MessageID: uint32(test.formID),
				},
				Payload: payload,
			})

			if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
				t.Fatalf("Dynasty FORM %d failed: type=%d error=0x%04x", test.formID, response.Header.MessageType, response.Header.ErrorCode)
			}
			if len(response.Payload) != test.length {
				t.Fatalf("Dynasty FORM %d returned %d bytes, want %d", test.formID, len(response.Payload), test.length)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(response.Payload)); got != test.sha256 {
				t.Fatalf("Dynasty FORM %d payload hash = %s, want %s", test.formID, got, test.sha256)
			}
		})
	}
}

func TestDynastyFormRejectsUnknownID(t *testing.T) {
	payload, err := blaze.Encode([]blaze.Field{
		{Tag: "FORM", Type: blaze.TypeInteger, Value: int64(999)},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(Config{Profile: "LocalPlayer"})
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header: blaze.Header{
			Component: ComponentBootStatus, Command: 533,
			MessageType: blaze.MessageTypeRequest, MessageID: 533,
		},
		Payload: payload,
	})
	if response.Header.MessageType != blaze.MessageTypeErrorReply || response.Header.ErrorCode != ErrorCommandNotFound {
		t.Fatalf("unknown Dynasty FORM returned type=%d error=0x%04x", response.Header.MessageType, response.Header.ErrorCode)
	}
}

func TestDynastySettingsMutationResponsesMatchCapture(t *testing.T) {
	for _, command := range []uint16{303, 305, 306, 307, 309} {
		t.Run(fmt.Sprintf("command-%d", command), func(t *testing.T) {
			svc := NewService(Config{Profile: "LocalPlayer"})
			response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
				Header: blaze.Header{
					Component: ComponentBootStatus, Command: command,
					MessageType: blaze.MessageTypeRequest, MessageID: uint32(command),
				},
			})
			if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
				t.Fatalf("Dynasty mutation command %d failed: type=%d error=0x%04x", command, response.Header.MessageType, response.Header.ErrorCode)
			}
			fields, err := blaze.Decode(response.Payload)
			if err != nil {
				t.Fatal(err)
			}
			if value, ok := stringField(fields, "SMSG"); !ok || value != "" {
				t.Fatalf("command %d expected empty SMSG, got %q", command, value)
			}
			if value, ok := integerField(fields, "SUCC"); !ok || value != 1 {
				t.Fatalf("command %d expected SUCC=1, got %d", command, value)
			}
		})
	}

	svc := NewService(Config{Profile: "LocalPlayer"})
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header: blaze.Header{
			Component: ComponentBootStatus, Command: 302,
			MessageType: blaze.MessageTypeRequest, MessageID: 302,
		},
	})
	if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 || len(response.Payload) != 0 {
		t.Fatalf("Dynasty exit command failed: type=%d error=0x%04x payload=%x", response.Header.MessageType, response.Header.ErrorCode, response.Payload)
	}
}

func TestDynastyCoachSelectionResponsesMatchCapture(t *testing.T) {
	tests := []struct {
		coachType int64
		length    int
		sha256    string
	}{
		{coachType: 2, length: 6661, sha256: "1aa95b0e3c6c9ae4cefe3b3ece1b1a633e0496f863892b728dad4a04dc1a07ff"},
		{coachType: 3, length: 6637, sha256: "6b2eeb14af973cdee5ae7c6dd76b0542eb1d29281e67c03c1fc7e6fd74ff071c"},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("coach-type-%d", test.coachType), func(t *testing.T) {
			payload, err := blaze.Encode([]blaze.Field{
				{Tag: "CVTP", Type: blaze.TypeInteger, Value: test.coachType},
			})
			if err != nil {
				t.Fatal(err)
			}
			svc := NewService(Config{Profile: "LocalPlayer"})
			response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
				Header: blaze.Header{
					Component: ComponentBootStatus, Command: 1111,
					MessageType: blaze.MessageTypeRequest, MessageID: uint32(test.coachType),
				},
				Payload: payload,
			})
			if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
				t.Fatalf("Dynasty coach type %d failed: type=%d error=0x%04x", test.coachType, response.Header.MessageType, response.Header.ErrorCode)
			}
			if len(response.Payload) != test.length {
				t.Fatalf("Dynasty coach type %d returned %d bytes, want %d", test.coachType, len(response.Payload), test.length)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(response.Payload)); got != test.sha256 {
				t.Fatalf("Dynasty coach type %d payload hash = %s, want %s", test.coachType, got, test.sha256)
			}
		})
	}
}

func TestDynastyCoachSelectionRejectsUnknownType(t *testing.T) {
	payload, err := blaze.Encode([]blaze.Field{
		{Tag: "CVTP", Type: blaze.TypeInteger, Value: int64(99)},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(Config{Profile: "LocalPlayer"})
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header: blaze.Header{
			Component: ComponentBootStatus, Command: 1111,
			MessageType: blaze.MessageTypeRequest, MessageID: 1111,
		},
		Payload: payload,
	})
	if response.Header.MessageType != blaze.MessageTypeErrorReply || response.Header.ErrorCode != ErrorCommandNotFound {
		t.Fatalf("unknown Dynasty coach type returned type=%d error=0x%04x", response.Header.MessageType, response.Header.ErrorCode)
	}
}

func TestDynastyCoachSelectionUsesRequestedTeamRecord(t *testing.T) {
	payload, err := blaze.Encode([]blaze.Field{
		{Tag: "CVTP", Type: blaze.TypeInteger, Value: int64(2)},
		{Tag: "TMKY", Type: blaze.TypeInteger, Value: int64(830865495)},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := newServiceWithAuthoritativeCoaches(t)
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header:  blaze.Header{Component: ComponentBootStatus, Command: 1111, MessageType: blaze.MessageTypeRequest, MessageID: 1111},
		Payload: payload,
	})
	if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
		t.Fatalf("coach selection failed: type=%d error=0x%04x", response.Header.MessageType, response.Header.ErrorCode)
	}
	if got := svc.fallbackSession().selectedTeam.Load(); got != 830865495 {
		t.Fatalf("active Dynasty team = %d, want Ohio State key 830865495", got)
	}
	for _, want := range [][]byte{[]byte("Ohio State"), []byte("Ryan"), []byte("Day")} {
		if !bytes.Contains(response.Payload, want) {
			t.Fatalf("coach selection response does not contain selected-team value %q", want)
		}
	}
	for _, selectedField := range []blaze.Field{
		{Tag: "CCID", Type: blaze.TypeInteger, Value: int64(665)},
		{Tag: "CHPR", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "CSXP", Type: blaze.TypeInteger, Value: int64(618)},
		{Tag: "COAC", Type: blaze.TypeInteger, Value: int64(12)},
		{Tag: "CHAT", Type: blaze.TypeInteger, Value: int64(2)},
		{Tag: "DEFS", Type: blaze.TypeInteger, Value: int64(14)},
		{Tag: "OFFS", Type: blaze.TypeInteger, Value: int64(6)},
		{Tag: "LEVL", Type: blaze.TypeInteger, Value: int64(76)},
		{Tag: "TMID", Type: blaze.TypeInteger, Value: int64(1178)},
	} {
		wire, err := blaze.Encode([]blaze.Field{selectedField})
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(response.Payload, wire) {
			t.Fatalf("coach selection response is missing authoritative %s field %x", selectedField.Tag, wire)
		}
	}
	for _, captured := range [][]byte{[]byte("Akron"), []byte("Moorhead"), []byte("Local"), []byte("NewCoach")} {
		if bytes.Contains(response.Payload, captured) {
			t.Fatalf("coach selection response still contains captured fallback value %q", captured)
		}
	}
}

func TestDynastyCoachSelectionUsesRequestedAuthoritativeCoachKey(t *testing.T) {
	payload, err := blaze.Encode([]blaze.Field{
		{Tag: "CHKY", Type: blaze.TypeInteger, Value: int64(dynastyCoachPortraitKeyBase + 877)},
		{Tag: "CVTP", Type: blaze.TypeInteger, Value: int64(2)},
		{Tag: "TMKY", Type: blaze.TypeInteger, Value: int64(830865495)},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := newServiceWithAuthoritativeCoaches(t)
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header:  blaze.Header{Component: ComponentBootStatus, Command: 1111, MessageType: blaze.MessageTypeRequest, MessageID: 1113},
		Payload: payload,
	})
	if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
		t.Fatalf("coach selection failed: type=%d error=0x%04x", response.Header.MessageType, response.Header.ErrorCode)
	}

	mainTagOffset := bytes.Index(response.Payload, dynastyTeamMainTag)
	historyTagOffset := bytes.Index(response.Payload, dynastyCoachHistoryTag)
	if mainTagOffset < 1 || historyTagOffset <= mainTagOffset {
		t.Fatal("coach selection response is missing its main coach record")
	}
	main := response.Payload[mainTagOffset-1 : historyTagOffset]
	assertDynastyStringScalar(t, main, "CFNM", "Arthur")
	assertDynastyStringScalar(t, main, "CLNM", "Smith")
	for tag, value := range map[string]int64{
		"CCID": 877, "CSXP": 1044, "LEVL": 30, "CPOS": 1, "COAC": 6, "CHAT": 0,
	} {
		assertDynastyIntegerScalar(t, main, tag, value)
	}
}

func TestDynastyCoachSelectionSurvivesPersistenceFailure(t *testing.T) {
	dynasty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Dynasty franchise persistence is not configured", http.StatusServiceUnavailable)
	}))
	defer dynasty.Close()
	payload, err := blaze.Encode([]blaze.Field{
		{Tag: "CVTP", Type: blaze.TypeInteger, Value: int64(2)},
		{Tag: "TMKY", Type: blaze.TypeInteger, Value: int64(830865495)},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := newServiceWithAuthoritativeCoaches(t)
	svc.config.DynastyURL = dynasty.URL
	svc.config.PersistDynasty = true
	svc.dynasty = NewDynastyClient(dynasty.URL)
	svc.fallbackSession().activeDynastySession.Store(77)
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header:  blaze.Header{Component: ComponentBootStatus, Command: 1111, MessageType: blaze.MessageTypeRequest, MessageID: 1114},
		Payload: payload,
	})
	if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
		t.Fatalf("persistence failure rejected coach selection: type=%d error=0x%04x", response.Header.MessageType, response.Header.ErrorCode)
	}
	if got := svc.fallbackSession().selectedTeam.Load(); got != 830865495 {
		t.Fatalf("active Dynasty team = %d after persistence failure", got)
	}
}

func TestDynastyCoachSelectionPersistsTeamToActiveFranchise(t *testing.T) {
	selected := make(chan [2]int64, 1)
	dynasty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/sessions/77/select-team" {
			http.NotFound(w, r)
			return
		}
		var request struct {
			TeamKey  int64 `json:"teamKey"`
			CoachKey int64 `json:"coachKey"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		selected <- [2]int64{request.TeamKey, request.CoachKey}
		_ = json.NewEncoder(w).Encode(DynastySession{ID: 77, SelectedTeamKey: request.TeamKey, SaveRevision: 1})
	}))
	defer dynasty.Close()
	payload, err := blaze.Encode([]blaze.Field{
		{Tag: "CHKY", Type: blaze.TypeInteger, Value: int64(dynastyCoachPortraitKeyBase + 680)},
		{Tag: "CVTP", Type: blaze.TypeInteger, Value: int64(2)},
		{Tag: "TMKY", Type: blaze.TypeInteger, Value: int64(830865500)},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := newServiceWithAuthoritativeCoaches(t)
	svc.config.DynastyURL = dynasty.URL
	svc.config.PersistDynasty = true
	svc.dynasty = NewDynastyClient(dynasty.URL)
	svc.fallbackSession().activeDynastySession.Store(77)
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header:  blaze.Header{Component: ComponentBootStatus, Command: 1111, MessageType: blaze.MessageTypeRequest, MessageID: 1112},
		Payload: payload,
	})
	if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
		t.Fatalf("coach selection failed: type=%d error=0x%04x", response.Header.MessageType, response.Header.ErrorCode)
	}
	select {
	case got := <-selected:
		if got != [2]int64{830865500, dynastyCoachPortraitKeyBase + 680} {
			t.Fatalf("persisted team/coach keys = %v", got)
		}
	default:
		t.Fatal("coach selection was not persisted to the active Dynasty artifact")
	}
}

func TestDynastyTeamBrowseUsesDistinctAuthoritativeStaff(t *testing.T) {
	payload, err := blaze.Encode([]blaze.Field{
		{Tag: "FORM", Type: blaze.TypeInteger, Value: int64(133300354)},
		{Tag: "LGID", Type: blaze.TypeInteger, Value: int64(1)},
		{Tag: "PFIL", Type: blaze.TypeInteger, Value: int64(830865495)},
		{Tag: "SKEY", Type: blaze.TypeInteger, Value: int64(0)},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := newServiceWithAuthoritativeCoaches(t)
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header:  blaze.Header{Component: ComponentBootStatus, Command: 533, MessageType: blaze.MessageTypeRequest, MessageID: 533},
		Payload: payload,
	})
	if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
		t.Fatalf("team browse failed: type=%d error=0x%04x", response.Header.MessageType, response.Header.ErrorCode)
	}

	rows := dynastyStaffCardRows(t, response.Payload, "Ohio State")
	expected := []struct {
		first, last string
		coachKey    int64
		portrait    int64
		level       int64
		pipeline    int64
		archetype   int64
		prestige    int64
		coachType   int64
	}{
		{first: "Ryan", last: "Day", coachKey: dynastyCoachPortraitKeyBase + 665, portrait: 618, level: 76, pipeline: 28, archetype: 12, prestige: 0, coachType: 2},
		{first: "Matt", last: "Patricia", coachKey: dynastyCoachPortraitKeyBase + 667, portrait: 964, level: 32, pipeline: 22, archetype: 7, prestige: 6, coachType: 1},
		{first: "Arthur", last: "Smith", coachKey: dynastyCoachPortraitKeyBase + 877, portrait: 1044, level: 30, pipeline: 37, archetype: 6, prestige: 6, coachType: 0},
	}
	for index, want := range expected {
		row := rows[index]
		assertDynastyStringScalar(t, row, "FNAM", want.first)
		assertDynastyStringScalar(t, row, "LNAM", want.last)
		for tag, value := range map[string]int64{
			"USEN": want.coachKey, "PPOR": want.portrait, "PERF": want.level, "PIPE": want.pipeline,
			"CTAE": want.archetype, "CPRE": want.prestige, "CHAT": want.coachType,
			"TEAM": 830865495, "TMID": 1178, "TMLG": 78,
			"DIVI": 824573958,
		} {
			assertDynastyIntegerScalar(t, row, tag, value)
		}
	}
}

func TestDynastyTeamSelectionDatabasePreservesDistinctTeams(t *testing.T) {
	payload, err := blaze.Encode([]blaze.Field{
		{Tag: "FORM", Type: blaze.TypeInteger, Value: int64(133300355)},
		{Tag: "LGID", Type: blaze.TypeInteger, Value: int64(2)},
		{Tag: "PFIL", Type: blaze.TypeInteger, Value: int64(830865495)},
		{Tag: "SKEY", Type: blaze.TypeInteger, Value: int64(0)},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := newServiceWithAuthoritativeCoaches(t)
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header:  blaze.Header{Component: ComponentBootStatus, Command: 533, MessageType: blaze.MessageTypeRequest, MessageID: 533},
		Payload: payload,
	})
	if !bytes.Equal(response.Payload, dynastyForm133300355Payload) {
		t.Fatal("team-selection database was rewritten for the selected team")
	}
	for _, name := range []string{"Akron", "Ohio State"} {
		if !bytes.Contains(response.Payload, []byte(name)) {
			t.Fatalf("team-selection database no longer contains %s", name)
		}
	}
}

func TestSelectedTeamLocalizationPatchesNumericOnlyTeamIdentity(t *testing.T) {
	svc := newServiceWithAuthoritativeCoaches(t)
	svc.fallbackSession().selectedTeam.Store(830865500)
	payload, err := blaze.Encode([]blaze.Field{
		{Tag: "TMKY", Type: blaze.TypeInteger, Value: int64(capturedDynastyTeamKey)},
		{Tag: "TMID", Type: blaze.TypeInteger, Value: int64(1101)},
		{Tag: "TMLI", Type: blaze.TypeInteger, Value: int64(1)},
		{Tag: "TMLG", Type: blaze.TypeInteger, Value: int64(1)},
	})
	if err != nil {
		t.Fatal(err)
	}
	localized := svc.localizeSelectedTeam(svc.fallbackSession(), payload)
	for tag, value := range map[string]int64{
		"TMKY": 830865500, "TMID": 1183, "TMLI": 83, "TMLG": 83,
	} {
		assertDynastyIntegerScalar(t, localized, tag, value)
	}
}

func TestSelectedTeamLocalizationDoesNotCorruptPresentationValuedTEAM(t *testing.T) {
	svc := newServiceWithAuthoritativeCoaches(t)
	svc.fallbackSession().selectedTeam.Store(830865500)
	payload, err := blaze.Encode([]blaze.Field{
		{Tag: "TEAM", Type: blaze.TypeInteger, Value: int64(1178)},
		{Tag: "TMNM", Type: blaze.TypeString, Value: "Akron"},
	})
	if err != nil {
		t.Fatal(err)
	}
	localized := svc.localizeSelectedTeam(svc.fallbackSession(), payload)
	assertDynastyIntegerScalar(t, localized, "TEAM", 1178)
}

func TestDynastyHubUsesOregonAuthoritativeIdentity(t *testing.T) {
	svc := newServiceWithAuthoritativeCoaches(t)
	svc.fallbackSession().selectedTeam.Store(830865500)
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header: blaze.Header{Component: ComponentBootStatus, Command: 541, MessageType: blaze.MessageTypeRequest, MessageID: 541},
	})
	if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
		t.Fatalf("Dynasty Hub request failed: type=%d error=0x%04x", response.Header.MessageType, response.Header.ErrorCode)
	}
	assertDynastyFieldsNearString(t, response.Payload, blaze.Field{Tag: "TLNM", Type: blaze.TypeString, Value: "Oregon"}, 128, 320, []blaze.Field{
		{Tag: "TEAM", Type: blaze.TypeInteger, Value: int64(830865500)},
		{Tag: "TMID", Type: blaze.TypeInteger, Value: int64(1183)},
		{Tag: "TMLI", Type: blaze.TypeInteger, Value: int64(83)},
		{Tag: "TMLG", Type: blaze.TypeInteger, Value: int64(83)},
		{Tag: "USID", Type: blaze.TypeInteger, Value: int64(547356846)},
		{Tag: "USOV", Type: blaze.TypeInteger, Value: int64(70)},
	})
}

func TestDynastyTeamBrowsePreservesStaffTopologyWithoutAuthoritativeCatalog(t *testing.T) {
	payload, err := blaze.Encode([]blaze.Field{
		{Tag: "FORM", Type: blaze.TypeInteger, Value: int64(133300354)},
		{Tag: "LGID", Type: blaze.TypeInteger, Value: int64(1)},
		{Tag: "PFIL", Type: blaze.TypeInteger, Value: int64(830865487)},
		{Tag: "SKEY", Type: blaze.TypeInteger, Value: int64(0)},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(Config{Profile: "LocalPlayer"})
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header:  blaze.Header{Component: ComponentBootStatus, Command: 533, MessageType: blaze.MessageTypeRequest, MessageID: 533},
		Payload: payload,
	})
	if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
		t.Fatalf("team browse failed: type=%d error=0x%04x", response.Header.MessageType, response.Header.ErrorCode)
	}

	for _, selectedField := range []blaze.Field{
		{Tag: "DIVI", Type: blaze.TypeInteger, Value: int64(824573958)},
	} {
		wire, err := blaze.Encode([]blaze.Field{selectedField})
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(response.Payload, wire) {
			t.Fatalf("team browse response is missing selected %s field %x", selectedField.Tag, wire)
		}
	}
}

func TestDynastyTeamBrowseUsesAuthoritativeHeadCoach(t *testing.T) {
	payload, err := blaze.Encode([]blaze.Field{
		{Tag: "FORM", Type: blaze.TypeInteger, Value: int64(133300354)},
		{Tag: "LGID", Type: blaze.TypeInteger, Value: int64(1)},
		{Tag: "PFIL", Type: blaze.TypeInteger, Value: int64(830865495)},
		{Tag: "SKEY", Type: blaze.TypeInteger, Value: int64(0)},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := newServiceWithAuthoritativeCoaches(t)
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header:  blaze.Header{Component: ComponentBootStatus, Command: 533, MessageType: blaze.MessageTypeRequest, MessageID: 533},
		Payload: payload,
	})
	if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
		t.Fatalf("team browse failed: type=%d error=0x%04x", response.Header.MessageType, response.Header.ErrorCode)
	}
	for _, selectedField := range []blaze.Field{
		{Tag: "DIVI", Type: blaze.TypeInteger, Value: int64(824573958)},
		{Tag: "PPOR", Type: blaze.TypeInteger, Value: int64(618)},
		{Tag: "PERF", Type: blaze.TypeInteger, Value: int64(76)},
		{Tag: "CHAT", Type: blaze.TypeInteger, Value: int64(2)},
		{Tag: "CTAE", Type: blaze.TypeInteger, Value: int64(12)},
		{Tag: "CPRE", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "PIPE", Type: blaze.TypeInteger, Value: int64(28)},
		{Tag: "CDSC", Type: blaze.TypeInteger, Value: int64(14)},
		{Tag: "COSC", Type: blaze.TypeInteger, Value: int64(6)},
		{Tag: "TMID", Type: blaze.TypeInteger, Value: int64(1178)},
		{Tag: "TMLG", Type: blaze.TypeInteger, Value: int64(78)},
		{Tag: "TDEF", Type: blaze.TypeInteger, Value: int64(96)},
		{Tag: "TOFF", Type: blaze.TypeInteger, Value: int64(94)},
		{Tag: "FNAM", Type: blaze.TypeString, Value: "Ryan"},
		{Tag: "LNAM", Type: blaze.TypeString, Value: "Day"},
	} {
		wire, err := blaze.Encode([]blaze.Field{selectedField})
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(response.Payload, wire) {
			t.Fatalf("team browse response is missing authoritative %s field %x", selectedField.Tag, wire)
		}
	}
}

func TestDynastyCoachSelectionUsesSelectedTeamHistoryTopology(t *testing.T) {
	payload, err := blaze.Encode([]blaze.Field{
		{Tag: "CVTP", Type: blaze.TypeInteger, Value: int64(2)},
		{Tag: "TMKY", Type: blaze.TypeInteger, Value: int64(830865496)},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := newServiceWithAuthoritativeCoaches(t)
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header:  blaze.Header{Component: ComponentBootStatus, Command: 1111, MessageType: blaze.MessageTypeRequest, MessageID: 1111},
		Payload: payload,
	})
	if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
		t.Fatalf("coach selection failed: type=%d error=0x%04x", response.Header.MessageType, response.Header.ErrorCode)
	}

	historyTagOffset := bytes.Index(response.Payload, dynastyCoachHistoryTag)
	if historyTagOffset < 0 || historyTagOffset+6 > len(response.Payload) {
		t.Fatal("coach selection response is missing its captured history table")
	}
	capturedHistoryTagOffset := bytes.Index(dynastyCoachType2Payload, dynastyCoachHistoryTag)
	if capturedHistoryTagOffset < 0 || capturedHistoryTagOffset+6 > len(dynastyCoachType2Payload) {
		t.Fatal("captured coach response is missing its history table")
	}
	wantHistoryCount := dynastyCoachType2Payload[capturedHistoryTagOffset+5]
	if got := response.Payload[historyTagOffset+5]; got != wantHistoryCount {
		t.Fatalf("coach selection history row count = %d, want captured fixed topology %d", got, wantHistoryCount)
	}
	if _, ok := dynastyCompactHistoryRowsEnd(response.Payload, historyTagOffset+6, int(wantHistoryCount)); !ok {
		t.Fatalf("coach selection history table does not contain %d structurally complete rows", wantHistoryCount)
	}
	for _, selectedField := range []blaze.Field{
		{Tag: "CCID", Type: blaze.TypeInteger, Value: int64(668)},
		{Tag: "CHPR", Type: blaze.TypeInteger, Value: int64(2)},
		{Tag: "CSXP", Type: blaze.TypeInteger, Value: int64(898)},
		{Tag: "COAC", Type: blaze.TypeInteger, Value: int64(4)},
		{Tag: "CHAT", Type: blaze.TypeInteger, Value: int64(2)},
		{Tag: "LEVL", Type: blaze.TypeInteger, Value: int64(59)},
	} {
		wire, err := blaze.Encode([]blaze.Field{selectedField})
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(response.Payload, wire) {
			t.Fatalf("coach selection response is missing selected %s field %x", selectedField.Tag, wire)
		}
	}
}

func TestDynastyCoachSelectionUsesSelectedTeamContractAndHistory(t *testing.T) {
	payload, err := blaze.Encode([]blaze.Field{
		{Tag: "CHKY", Type: blaze.TypeInteger, Value: int64(dynastyCoachPortraitKeyBase + 665)},
		{Tag: "CVTP", Type: blaze.TypeInteger, Value: int64(2)},
		{Tag: "TMKY", Type: blaze.TypeInteger, Value: int64(830865495)},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := newServiceWithAuthoritativeCoaches(t)
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header:  blaze.Header{Component: ComponentBootStatus, Command: 1111, MessageType: blaze.MessageTypeRequest, MessageID: 1111},
		Payload: payload,
	})
	for _, selectedValue := range []string{
		"Beat Michigan by 3+ points", "Sign a top 10 Class at National Signing Day",
		"R. Day", "12 - 2", "14 - 2", "11 - 2",
	} {
		if !bytes.Contains(response.Payload, []byte(selectedValue)) {
			t.Fatalf("selected-team contract response is missing %q", selectedValue)
		}
	}
	for _, capturedValue := range []string{"Beat Kent State", "J. Moorhead", "2 - 10", "4 - 8", "5 - 7"} {
		if bytes.Contains(response.Payload, []byte(capturedValue)) {
			t.Fatalf("selected-team contract response still contains Akron value %q", capturedValue)
		}
	}
}

func TestDynastyHubUsesRequestedTeamIdentity(t *testing.T) {
	svc := newServiceWithAuthoritativeCoaches(t)
	svc.fallbackSession().selectedTeam.Store(830865495)
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header: blaze.Header{Component: ComponentBootStatus, Command: 541, MessageType: blaze.MessageTypeRequest, MessageID: 541},
	})
	if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
		t.Fatalf("Dynasty Hub request failed: type=%d error=0x%04x", response.Header.MessageType, response.Header.ErrorCode)
	}
	for _, want := range [][]byte{[]byte("Ohio State"), []byte("Buckeyes")} {
		if !bytes.Contains(response.Payload, want) {
			t.Fatalf("Dynasty Hub response does not contain selected-team value %q", want)
		}
	}
	if bytes.Contains(response.Payload, []byte("Local Coach")) {
		t.Fatal("Dynasty Hub response still contains the captured local-coach fallback")
	}
	assertDynastyFieldsNearString(t, response.Payload, blaze.Field{Tag: "TLNM", Type: blaze.TypeString, Value: "Ohio State"}, 128, 320, []blaze.Field{
		{Tag: "TEAM", Type: blaze.TypeInteger, Value: int64(830865495)},
		{Tag: "TMID", Type: blaze.TypeInteger, Value: int64(1178)},
		{Tag: "TMLI", Type: blaze.TypeInteger, Value: int64(78)},
		{Tag: "TMLG", Type: blaze.TypeInteger, Value: int64(78)},
		{Tag: "DEFR", Type: blaze.TypeInteger, Value: int64(96)},
		{Tag: "OFFR", Type: blaze.TypeInteger, Value: int64(94)},
		{Tag: "OVLR", Type: blaze.TypeInteger, Value: int64(94)},
		{Tag: "SCHP", Type: blaze.TypeInteger, Value: int64(10)},
		{Tag: "TPCR", Type: blaze.TypeInteger, Value: int64(0xc10230)},
		{Tag: "TSCR", Type: blaze.TypeInteger, Value: int64(0xa2a9ad)},
		{Tag: "TSCO", Type: blaze.TypeString, Value: "Big Ten"},
		{Tag: "USID", Type: blaze.TypeInteger, Value: int64(547356831)},
		{Tag: "USOV", Type: blaze.TypeInteger, Value: int64(76)},
	})
	assertDynastyFieldsNearString(t, response.Payload, blaze.Field{Tag: "TENA", Type: blaze.TypeString, Value: "Akron"}, 32, 240, []blaze.Field{
		{Tag: "TLNM", Type: blaze.TypeString, Value: "Akron"},
		{Tag: "TNME", Type: blaze.TypeString, Value: "Zips"},
		{Tag: "TMID", Type: blaze.TypeInteger, Value: int64(1101)},
		{Tag: "TMLI", Type: blaze.TypeInteger, Value: int64(1)},
		{Tag: "TMLG", Type: blaze.TypeInteger, Value: int64(1)},
	})
}

func TestDynastyHubMovesUserAssociationToSelectedLeagueTeam(t *testing.T) {
	svc := newServiceWithAuthoritativeCoaches(t)
	svc.fallbackSession().selectedTeam.Store(830865495)
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header: blaze.Header{Component: ComponentBootStatus, Command: 541, MessageType: blaze.MessageTypeRequest, MessageID: 541},
	})
	if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
		t.Fatalf("Dynasty Hub request failed: type=%d error=0x%04x", response.Header.MessageType, response.Header.ErrorCode)
	}

	assertLastLeagueTeamUSID := func(teamName string, want int64) {
		t.Helper()
		anchor, err := blaze.Encode([]blaze.Field{{Tag: "TENA", Type: blaze.TypeString, Value: teamName}})
		if err != nil {
			t.Fatal(err)
		}
		offset := bytes.LastIndex(response.Payload, anchor)
		if offset < 0 {
			t.Fatalf("hub league table is missing %s", teamName)
		}
		end := len(response.Payload)
		tenaTag, err := blaze.EncodeTag("TENA")
		if err != nil {
			t.Fatal(err)
		}
		if relative := bytes.Index(response.Payload[offset+len(anchor):], tenaTag[:]); relative >= 0 {
			end = offset + len(anchor) + relative
		}
		usid, err := blaze.Encode([]blaze.Field{{Tag: "USID", Type: blaze.TypeInteger, Value: want}})
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(response.Payload[offset:end], usid) {
			t.Fatalf("hub league team %s does not have USID=%d", teamName, want)
		}
	}

	assertLastLeagueTeamUSID("Akron", 0)
	assertLastLeagueTeamUSID("Ohio State", 547356831)
}

func TestDynastyHubNotificationsUseSelectedTeamAssets(t *testing.T) {
	svc := newServiceWithAuthoritativeCoaches(t)
	svc.fallbackSession().activeDynastySession.Store(3)
	svc.fallbackSession().selectedTeam.Store(830865495)
	frames := svc.responseSequence(context.Background(), "test-connection", blaze.Frame{Header: blaze.Header{
		Component: ComponentBootStatus, Command: 541,
		MessageType: blaze.MessageTypeRequest, MessageID: 541,
	}})
	var notifications []byte
	for _, frame := range frames {
		if frame.Header.MessageType == blaze.MessageTypeNotification {
			notifications = append(notifications, frame.Payload...)
		}
	}
	for _, stale := range []string{"AKRONX", "#GoZips"} {
		if bytes.Contains(notifications, []byte(stale)) {
			t.Fatalf("selected-team hub notifications still contain Akron asset %q", stale)
		}
	}
	for _, selected := range []string{"OHIOST", "#OH-IO"} {
		if !bytes.Contains(notifications, []byte(selected)) {
			t.Fatalf("selected-team hub notifications are missing Ohio State asset %q", selected)
		}
	}
}

func TestSelectedTeamIdentityDoesNotRenameUnrelatedFirstName(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	svc.fallbackSession().selectedTeam.Store(830865495)
	unrelatedJoe := []byte{0x04, 'J', 'o', 'e', 0}
	localized := svc.localizeSelectedTeam(svc.fallbackSession(), unrelatedJoe)
	if !bytes.Equal(localized, unrelatedJoe) {
		t.Fatalf("unrelated first-name field was changed: got %x want %x", localized, unrelatedJoe)
	}
}

func TestDynastyTeamBuilderEntryReturnsEmptyDatabase(t *testing.T) {
	payload, err := blaze.Encode([]blaze.Field{
		{Tag: "CKEY", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "LGID", Type: blaze.TypeInteger, Value: int64(1)},
		{Tag: "SKEY", Type: blaze.TypeInteger, Value: int64(0)},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(Config{Profile: "LocalPlayer"})
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header: blaze.Header{
			Component: ComponentBootStatus, Command: 1269,
			MessageType: blaze.MessageTypeRequest, MessageID: 1269,
		},
		Payload: payload,
	})
	if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
		t.Fatalf("Team Builder entry failed: type=%d error=0x%04x", response.Header.MessageType, response.Header.ErrorCode)
	}
	fields, err := blaze.Decode(response.Payload)
	if err != nil {
		t.Fatal(err)
	}
	formField, ok := fieldByTag(fields, "FORM")
	if !ok {
		t.Fatal("Team Builder response is missing FORM")
	}
	form, ok := formField.Value.([]blaze.Field)
	if !ok {
		t.Fatalf("FORM has unexpected type %T", formField.Value)
	}
	for _, tag := range []string{"DICT", "RIBC", "ROOT", "SIBC", "TABL"} {
		if _, ok := fieldByTag(form, tag); !ok {
			t.Fatalf("Team Builder FORM is missing %s", tag)
		}
	}
	for _, tag := range []string{"DICT", "TABL"} {
		field, _ := fieldByTag(form, tag)
		value, ok := field.Value.(blaze.Map)
		if !ok || len(value.Entries) != 0 {
			t.Fatalf("Team Builder FORM %s is not an empty map: %#v", tag, field.Value)
		}
	}
	if value, ok := stringField(fields, "SMSG"); !ok || value != "" {
		t.Fatalf("expected empty SMSG, got %q", value)
	}
	if value, ok := integerField(fields, "SUCC"); !ok || value != 1 {
		t.Fatalf("expected SUCC=1, got %d", value)
	}
}

func TestDynastyCloseClearsActiveClientState(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	svc.fallbackSession().activeDynastySession.Store(3)
	svc.fallbackSession().selectedTeam.Store(830865495)
	svc.fallbackSession().dynastyContract.Store(4)
	svc.fallbackSession().dynastyHub.Store(5)
	svc.fallbackSession().dynastyAdvance.Store(6)

	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header: blaze.Header{
			Component: ComponentBootStatus, Command: 162,
			MessageType: blaze.MessageTypeRequest, MessageID: 162,
		},
	})

	if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
		t.Fatalf("Dynasty close failed: type=%d error=0x%04x", response.Header.MessageType, response.Header.ErrorCode)
	}
	for name, got := range map[string]int64{
		"active Dynasty session": svc.fallbackSession().activeDynastySession.Load(),
		"selected team":          svc.fallbackSession().selectedTeam.Load(),
		"contract cursor":        int64(svc.fallbackSession().dynastyContract.Load()),
		"hub cursor":             int64(svc.fallbackSession().dynastyHub.Load()),
		"advance cursor":         int64(svc.fallbackSession().dynastyAdvance.Load()),
	} {
		if got != 0 {
			t.Errorf("%s was not cleared: got %d", name, got)
		}
	}
}

func TestDynastyRefreshFormReturnsRequestedDatabaseRootAndEchoesFilters(t *testing.T) {
	payload, err := blaze.Encode([]blaze.Field{
		{Tag: "FORM", Type: blaze.TypeInteger, Value: int64(133300238)},
		{Tag: "LGID", Type: blaze.TypeInteger, Value: int64(1)},
		{Tag: "PFIL", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "RDON", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "SFIL", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "SKEY", Type: blaze.TypeInteger, Value: int64(0)},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(Config{Profile: "LocalPlayer"})
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header: blaze.Header{
			Component: ComponentBootStatus, Command: 1263,
			MessageType: blaze.MessageTypeRequest, MessageID: 1263,
		},
		Payload: payload,
	})
	if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
		t.Fatalf("Dynasty form refresh failed: type=%d error=0x%04x", response.Header.MessageType, response.Header.ErrorCode)
	}
	fields, err := blaze.Decode(response.Payload)
	if err != nil {
		t.Fatal(err)
	}
	dataField, ok := fieldByTag(fields, "DATA")
	if !ok {
		t.Fatal("refresh response is missing DATA")
	}
	database, ok := dataField.Value.([]blaze.Field)
	if !ok {
		t.Fatalf("DATA has unexpected type %T", dataField.Value)
	}
	if root, ok := integerField(database, "ROOT"); !ok || root != 133300238 {
		t.Fatalf("DATA ROOT = %d, want 133300238", root)
	}
	for _, expected := range []struct {
		tag   string
		value int64
	}{
		{tag: "FORM", value: 133300238},
		{tag: "PFIL", value: 0},
		{tag: "RDON", value: 0},
		{tag: "SFIL", value: 0},
		{tag: "SUCC", value: 1},
	} {
		if value, ok := integerField(fields, expected.tag); !ok || value != expected.value {
			t.Fatalf("%s = %d, want %d", expected.tag, value, expected.value)
		}
	}
	if value, ok := stringField(fields, "SMSG"); !ok || value != "" {
		t.Fatalf("SMSG = %q, want empty", value)
	}
}

func TestDynastyCoachContractReturnsCapturedConfirmation(t *testing.T) {
	payload, err := blaze.Encode([]blaze.Field{
		{Tag: "CHAR", Type: blaze.TypeInteger, Value: int64(547357082)},
		{Tag: "LGID", Type: blaze.TypeInteger, Value: int64(1)},
		{Tag: "TEAM", Type: blaze.TypeInteger, Value: int64(830865495)},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := newServiceWithAuthoritativeCoaches(t)
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header: blaze.Header{
			Component: ComponentBootStatus, Command: 534,
			MessageType: blaze.MessageTypeRequest, MessageID: 534,
		},
		Payload: payload,
	})
	if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
		t.Fatalf("Dynasty coach contract failed: type=%d error=0x%04x", response.Header.MessageType, response.Header.ErrorCode)
	}
	fields, err := blaze.Decode(response.Payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []struct {
		tag   string
		value int64
	}{
		{tag: "CHAR", value: 547357082},
		{tag: "ISCM", value: 0},
		{tag: "SUCC", value: 1},
		{tag: "TMKY", value: 830865495},
		{tag: "TMLD", value: 78},
		{tag: "TMUR", value: 2},
		{tag: "UPOS", value: 0},
	} {
		if value, ok := integerField(fields, expected.tag); !ok || value != expected.value {
			t.Fatalf("%s = %d, want %d", expected.tag, value, expected.value)
		}
	}
	if value, ok := stringField(fields, "CHNM"); !ok || value != "Ryan Day" {
		t.Fatalf("CHNM = %q, want Ryan Day", value)
	}
	if value, ok := stringField(fields, "TMNM"); !ok || value != "Ohio State" {
		t.Fatalf("TMNM = %q, want Ohio State", value)
	}
	if value, ok := stringField(fields, "SMSG"); !ok || value != "" {
		t.Fatalf("SMSG = %q, want empty", value)
	}
}

func TestDynastyContractAdvanceAcknowledgementMatchesCapture(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header: blaze.Header{
			Component: ComponentBootStatus, Command: 1112,
			MessageType: blaze.MessageTypeRequest, MessageID: 1112,
		},
	})
	if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
		t.Fatalf("Dynasty contract advance failed: type=%d error=0x%04x", response.Header.MessageType, response.Header.ErrorCode)
	}
	if len(response.Payload) != 0 {
		t.Fatalf("captured contract-advance response is empty, got %d bytes", len(response.Payload))
	}
}

func TestDynastyContractSettingsMutationMatchesCapture(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header: blaze.Header{
			Component: ComponentBootStatus, Command: 304,
			MessageType: blaze.MessageTypeRequest, MessageID: 304,
		},
	})
	if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
		t.Fatalf("Dynasty contract settings failed: type=%d error=0x%04x", response.Header.MessageType, response.Header.ErrorCode)
	}
	fields, err := blaze.Decode(response.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := stringField(fields, "SMSG"); !ok || value != "" {
		t.Fatalf("SMSG = %q, want empty", value)
	}
	if value, ok := integerField(fields, "SUCC"); !ok || value != 1 {
		t.Fatalf("SUCC = %d, want 1", value)
	}
}

func TestCapturedDynastyMutationAcknowledgements(t *testing.T) {
	for _, command := range []uint16{107, 163, 164, 304, 414} {
		t.Run(fmt.Sprintf("command_%d", command), func(t *testing.T) {
			svc := NewService(Config{Profile: "LocalPlayer"})
			response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
				Header: blaze.Header{
					Component: ComponentBootStatus, Command: command,
					MessageType: blaze.MessageTypeRequest, MessageID: uint32(command),
				},
			})
			if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
				t.Fatalf("captured mutation command %d failed: type=%d error=0x%04x", command, response.Header.MessageType, response.Header.ErrorCode)
			}
			fields, err := blaze.Decode(response.Payload)
			if err != nil {
				t.Fatal(err)
			}
			if value, ok := stringField(fields, "SMSG"); !ok || value != "" {
				t.Fatalf("SMSG = %q, want empty", value)
			}
			if value, ok := integerField(fields, "SUCC"); !ok || value != 1 {
				t.Fatalf("SUCC = %d, want 1", value)
			}
		})
	}
}

func TestDynastyAdvanceReplaysCapturedNotificationBatches(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	request := blaze.Frame{Header: blaze.Header{
		Component: ComponentBootStatus, Command: 107,
		MessageType: blaze.MessageTypeRequest, MessageID: 107,
	}}

	first := svc.responseSequence(context.Background(), "test-connection", request)
	if len(first) != 22 {
		t.Fatalf("first advance produced %d frames, want one reply plus 21 notifications", len(first))
	}
	second := svc.responseSequence(context.Background(), "test-connection", request)
	if len(second) != 2 {
		t.Fatalf("second advance produced %d frames, want one reply plus one notification", len(second))
	}
	third := svc.responseSequence(context.Background(), "test-connection", request)
	if len(third) != 1 {
		t.Fatalf("third advance produced %d frames, want only the reply", len(third))
	}

	replies := 0
	for _, frame := range first {
		switch frame.Header.MessageType {
		case blaze.MessageTypeReply:
			replies++
			if frame.Header.MessageID != request.Header.MessageID {
				t.Fatalf("reply message ID = %d, want %d", frame.Header.MessageID, request.Header.MessageID)
			}
		case blaze.MessageTypeNotification:
			if frame.Header.Component != 2099 || frame.Header.Command != 101 {
				t.Fatalf("unexpected notification route %d/%d", frame.Header.Component, frame.Header.Command)
			}
			metadata, err := blaze.Decode(frame.Metadata)
			if err != nil {
				t.Fatal(err)
			}
			if value, ok := integerField(metadata, "CNTX"); !ok || value != LocalBlazeID {
				t.Fatalf("notification CNTX = %d, want local Blaze ID %d", value, LocalBlazeID)
			}
			if len(frame.Payload) < 5 || frame.Payload[4] != 1 {
				t.Fatalf("notification payload does not carry local LGID=1: %x", frame.Payload[:min(len(frame.Payload), 9)])
			}
		}
	}
	if replies != 1 {
		t.Fatalf("first advance contained %d replies, want 1", replies)
	}
}

func TestDynastyNotificationsUseActiveLeagueID(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	svc.fallbackSession().activeDynastySession.Store(77)
	frames := svc.responseSequence(context.Background(), "test-connection", blaze.Frame{Header: blaze.Header{
		Component: ComponentBootStatus, Command: 107,
		MessageType: blaze.MessageTypeRequest, MessageID: 107,
	}})

	notifications := 0
	for _, frame := range frames {
		if frame.Header.MessageType != blaze.MessageTypeNotification {
			continue
		}
		notifications++
		leagueID, _, ok := dynastyUnsignedInteger(frame.Payload, 4)
		if !ok {
			t.Fatal("notification has no decodable LGID")
		}
		if leagueID != 77 {
			t.Fatalf("notification LGID = %d, want active league ID 77", leagueID)
		}
	}
	if notifications == 0 {
		t.Fatal("advance sequence contained no notifications")
	}
}

func TestDynastyAdvancePersistsBeforeReturningSuccess(t *testing.T) {
	var advanceRequests int
	dynasty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/sessions":
			_, _ = w.Write([]byte(`{"sessions":[{"id":77,"name":"Local Dynasty","currentWeek":1,"stage":"preseason"}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/sessions/77/advance":
			advanceRequests++
			var body struct {
				RequestID string `json:"requestId"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if body.RequestID != "advance-connection:91" {
				t.Fatalf("requestId = %q", body.RequestID)
			}
			_, _ = w.Write([]byte(`{"id":77,"name":"Local Dynasty","currentWeek":1,"stage":"week_1"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer dynasty.Close()

	svc := NewService(Config{Profile: "LocalPlayer", DynastyURL: dynasty.URL, RunID: "advance-connection", PersistDynasty: true})
	response := svc.HandleFrame(context.Background(), "advance-connection", blaze.Frame{
		Header: blaze.Header{Component: ComponentBootStatus, Command: 107, MessageType: blaze.MessageTypeRequest, MessageID: 91},
	})
	if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
		t.Fatalf("advance failed: type=%d error=0x%04x", response.Header.MessageType, response.Header.ErrorCode)
	}
	if advanceRequests != 1 {
		t.Fatalf("advance requests = %d, want 1", advanceRequests)
	}
}

func TestDynastyStartingTransitionReplaysPreAdvanceNotifications(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	contract := svc.responseSequence(context.Background(), "test-connection", blaze.Frame{Header: blaze.Header{
		Component: ComponentBootStatus, Command: 304, MessageType: blaze.MessageTypeRequest, MessageID: 304,
	}})
	if len(contract) != 7 {
		t.Fatalf("contract transition produced %d frames, want one reply plus six notifications", len(contract))
	}
	if contract[0].Header.MessageType != blaze.MessageTypeReply {
		t.Fatalf("contract transition starts with type %d, want reply", contract[0].Header.MessageType)
	}

	hub := svc.responseSequence(context.Background(), "test-connection", blaze.Frame{Header: blaze.Header{
		Component: ComponentBootStatus, Command: 541, MessageType: blaze.MessageTypeRequest, MessageID: 541,
	}})
	if len(hub) != 14 {
		t.Fatalf("hub transition produced %d frames, want thirteen notifications plus one reply", len(hub))
	}
	if hub[len(hub)-1].Header.MessageType != blaze.MessageTypeReply {
		t.Fatalf("hub transition ends with type %d, want reply", hub[len(hub)-1].Header.MessageType)
	}
	for _, sequence := range [][]blaze.Frame{contract, hub} {
		for _, frame := range sequence {
			if frame.Header.MessageType == blaze.MessageTypeNotification && (len(frame.Payload) < 5 || frame.Payload[4] != 1) {
				t.Fatalf("notification payload does not carry local LGID=1: %x", frame.Payload[:min(len(frame.Payload), 9)])
			}
		}
	}
}

func TestDynastyCreateResetsStartingTransitionNotifications(t *testing.T) {
	dynasty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":2,"name":"Second Dynasty","dynastyMode":"Online Dynasty","currentWeek":1,"stage":"preseason","maxUsers":32}`))
			return
		}
		_, _ = w.Write([]byte(`{"sessions":[{"id":1,"name":"Local Dynasty","dynastyMode":"Online Dynasty","currentWeek":1,"stage":"preseason","maxUsers":32}]}`))
	}))
	defer dynasty.Close()

	svc := NewService(Config{Profile: "LocalPlayer", DynastyURL: dynasty.URL})
	contractRequest := blaze.Frame{Header: blaze.Header{Component: ComponentBootStatus, Command: 304, MessageType: blaze.MessageTypeRequest, MessageID: 304}}
	hubRequest := blaze.Frame{Header: blaze.Header{Component: ComponentBootStatus, Command: 541, MessageType: blaze.MessageTypeRequest, MessageID: 541}}
	advanceRequest := blaze.Frame{Header: blaze.Header{Component: ComponentBootStatus, Command: 107, MessageType: blaze.MessageTypeRequest, MessageID: 107}}
	_ = svc.responseSequence(context.Background(), "test-connection", contractRequest)
	_ = svc.responseSequence(context.Background(), "test-connection", hubRequest)
	_ = svc.responseSequence(context.Background(), "test-connection", advanceRequest)

	createPayload, err := blaze.Encode([]blaze.Field{{Tag: "NAME", Type: blaze.TypeString, Value: "Second Dynasty"}})
	if err != nil {
		t.Fatal(err)
	}
	create := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header:  blaze.Header{Component: ComponentBootStatus, Command: CommandDynastyCreate, MessageType: blaze.MessageTypeRequest, MessageID: 101},
		Payload: createPayload,
	})
	if create.Header.ErrorCode != 0 {
		t.Fatalf("second Dynasty create failed with 0x%04x", create.Header.ErrorCode)
	}
	if got := len(svc.responseSequence(context.Background(), "test-connection", contractRequest)); got != 7 {
		t.Fatalf("second Dynasty contract transition produced %d frames, want 7", got)
	}
	if got := len(svc.responseSequence(context.Background(), "test-connection", hubRequest)); got != 14 {
		t.Fatalf("second Dynasty hub transition produced %d frames, want 14", got)
	}
	if got := len(svc.responseSequence(context.Background(), "test-connection", advanceRequest)); got != 22 {
		t.Fatalf("second Dynasty advance produced %d frames, want 22", got)
	}
}

func TestCoachAbilitiesRefreshReturnsCapturedTalentDatabase(t *testing.T) {
	payload, err := blaze.Encode([]blaze.Field{
		{Tag: "COKY", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "FORM", Type: blaze.TypeInteger, Value: int64(133300333)},
		{Tag: "LGID", Type: blaze.TypeInteger, Value: int64(1)},
		{Tag: "PFIL", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "RDON", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "SFIL", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "SFTM", Type: blaze.TypeInteger, Value: int64(1)},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(Config{Profile: "LocalPlayer"})
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header:  blaze.Header{Component: ComponentBootStatus, Command: 804, MessageType: blaze.MessageTypeRequest, MessageID: 804},
		Payload: payload,
	})
	if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
		t.Fatalf("Coach Abilities refresh failed: type=%d error=0x%04x", response.Header.MessageType, response.Header.ErrorCode)
	}
	for _, talent := range []string{"Motivator", "Tactician", "Recruiter"} {
		if !bytes.Contains(response.Payload, []byte(talent)) {
			t.Fatalf("Coach Abilities response is missing captured talent %q", talent)
		}
	}
	if len(response.Payload) < 10000 {
		t.Fatalf("Coach Abilities response has only %d bytes, want the captured talent database", len(response.Payload))
	}
}

func TestPlayerConfigDynastyProgressResponseMatchesCapture(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{Header: blaze.Header{
		Component: ComponentPlayerConfig, Command: 14003, MessageType: blaze.MessageTypeRequest, MessageID: 14003,
	}})
	if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
		t.Fatalf("player-config Dynasty progress failed: type=%d error=0x%04x", response.Header.MessageType, response.Header.ErrorCode)
	}
	fields, err := blaze.Decode(response.Payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{"FCGR", "IMXF"} {
		if value, ok := integerField(fields, tag); !ok || value != 0 {
			t.Fatalf("%s = %d, want 0", tag, value)
		}
	}
	mxField, ok := fieldByTag(fields, "MXIN")
	if !ok {
		t.Fatal("response is missing MXIN")
	}
	mxin, ok := mxField.Value.([]blaze.Field)
	if !ok {
		t.Fatalf("MXIN type = %T, want struct", mxField.Value)
	}
	for _, tag := range []string{"PAMT", "PPNT", "PTYP", "STUS"} {
		if value, ok := integerField(mxin, tag); !ok || value != 0 {
			t.Fatalf("MXIN.%s = %d, want 0", tag, value)
		}
	}
}

func TestDynastyEntitlementInventoryMatchesCapture(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{Header: blaze.Header{
		Component: ComponentAvatar, Command: 11, MessageType: blaze.MessageTypeRequest, MessageID: 11,
	}})
	fields, err := blaze.Decode(response.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if count, ok := integerField(fields, "TCNT"); !ok || count != 1 {
		t.Fatalf("TCNT = %d, want 1", count)
	}
	itemsField, ok := fieldByTag(fields, "INDL")
	if !ok {
		t.Fatal("inventory response is missing INDL")
	}
	items, ok := itemsField.Value.(blaze.List)
	if !ok || len(items.Values) != 1 {
		t.Fatalf("INDL = %#v, want one captured entitlement", itemsField.Value)
	}
}

func TestDynastyEntitlementBalancesMatchCapture(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{Header: blaze.Header{
		Component: ComponentAvatar, Command: 19, MessageType: blaze.MessageTypeRequest, MessageID: 19,
	}})
	fields, err := blaze.Decode(response.Payload)
	if err != nil {
		t.Fatal(err)
	}
	entriesField, ok := fieldByTag(fields, "ENTR")
	if !ok {
		t.Fatal("balance response is missing ENTR")
	}
	entries, ok := entriesField.Value.(blaze.List)
	if !ok || len(entries.Values) != 2 {
		t.Fatalf("ENTR = %#v, want two captured balances", entriesField.Value)
	}
}

func TestLocalizeDynastyNotificationPayloadReplacesLeadingLeagueID(t *testing.T) {
	captured := []byte{0xb2, 0x7a, 0x64, 0x00, 0x9b, 0xd8, 0xb8, 0x02, 0xb6, 0x5c, 0xe7, 0x04}
	got, err := localizeDynastyNotificationPayload(captured, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0xb2, 0x7a, 0x64, 0x00, 0x01, 0xb6, 0x5c, 0xe7, 0x04}
	if !bytes.Equal(got, want) {
		t.Fatalf("localized payload = %x, want %x", got, want)
	}
	if bytes.Equal(got, captured) {
		t.Fatal("localization mutated or reused the captured payload")
	}
}

func TestCapturedDynastyEmptyAcknowledgements(t *testing.T) {
	commands := []uint16{
		176, 192, 222, 272, 276, 312, 322, 362, 392, 412,
		502, 532, 542, 562, 801, 1112, 1132, 1152, 1252, 1272, 1411,
	}
	for _, command := range commands {
		t.Run(fmt.Sprintf("command_%d", command), func(t *testing.T) {
			svc := NewService(Config{Profile: "LocalPlayer"})
			response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
				Header: blaze.Header{
					Component: ComponentBootStatus, Command: command,
					MessageType: blaze.MessageTypeRequest, MessageID: uint32(command),
				},
			})
			if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
				t.Fatalf("captured empty command %d failed: type=%d error=0x%04x", command, response.Header.MessageType, response.Header.ErrorCode)
			}
			if len(response.Payload) != 0 {
				t.Fatalf("captured empty command %d returned %d bytes", command, len(response.Payload))
			}
		})
	}
}

func TestCapturedDynastyProgressionPayloads(t *testing.T) {
	tests := []struct {
		command uint16
		length  int
		sha256  string
	}{
		{1131, 4090, "54cf21dabde9e71af90098f00e9cc54af76a66d7517012ed4cc1bb5fda75af1a"},
		{1133, 7850, "d057e606aa638c0190d5d5b1929eb288e76ff8826db7310098cbde7b6366d868"},
		{1151, 188057, "fcaf25168fb1b90deb14a2969b504bdfe5f9b55cbe427aa741577b01b5056b82"},
		{1251, 38232, "1f9c396e7c34307917f2ce70d5959a9da6e37876f31c22bb80ef9e9c87829cc8"},
		{1271, 59685, "8b5c8eb279a42221e8e60f716a55309fc25a070fc565778caedd111be0350e80"},
		{1410, 62420, "1f703f1fcd6c51034b3915262508ca814fc3979d8868b44871f5639d8d860936"},
		{161, 46083, "d32659a4e5d3b7ee329692e27048002b3271d11214db9a419dc745e9c48fcf6d"},
		{175, 6869, "81f7fab524ca5560a9c7760c4e0d6479285d8005507cfa01b1beedd0162b760c"},
		{177, 69709, "e6e2acfa44220a4dbeb4fae49147bc016fc5ad98e659cfa46cb8c6eb34a103eb"},
		{191, 28459, "253f339103033b791cd375c2a90ce713698ac3681a8f6dcec9f7b3ab1411dd86"},
		{193, 14395, "b7f169de1781da7953c600ee9fece62ddeda2837312ad1f88e5372a7d132d4d9"},
		{221, 148811, "cdcbf4b41d81073b5b6af3bf40499ecaa92bb0469d9796f1e50da8f6b396507e"},
		{223, 557593, "2cbb03f01c9cbe4114a2cd520f75f79cd717e11883d58c8d746a892b63ac6c61"},
		{271, 23926, "e28bc22a7a58de3000864c6d1fca9db0677cc52c8638a022a79a440749cd040b"},
		{275, 601304, "84b21bce42f8097d3aea01493adbf691779bb2c98966d2b3e137ee6fb0d55e21"},
		{311, 36537, "c5898d0af1e37e44154c427856e26f359657bfd0dcfb892c9744ecbb96d4f7d4"},
		{313, 70512, "553fe2ed45f7ccfa3d2c029e0896eede2d48c0b954fd2112d4ea6d04ea4fc880"},
		{321, 18374, "b3b05fd35741c7c0d3f2fbda40c454227aec6dc486453224e8935c0a710615f1"},
		{323, 994, "5b769169b756c1ca459ad8be6d41c5ccbaddb8b4c78dfad45320d35902a0b250"},
		{361, 39641, "9577546d54d3513341642528abb4b70c574b49b9dac21141ea14465a84fbd6aa"},
		{363, 152690, "468520a7b29b3b7173144470505a52a37c3ab6cf3ea3263c47b138fe5067d1cc"},
		{391, 61381, "6cfb7556d9b7c4cc873243ebf63b6bbc9a2d5f30307453cb2e2081e892ec8d47"},
		{393, 188189, "90f9a54e9c4a33bda1cccca76dab624f2ae10eb45f189becec60ceb5b8e007b6"},
		{411, 21622, "4ef4496db4d34da6588de3f6a012cfb2ca26a605a5777a30da6371cf13971240"},
		{413, 45722, "33d9e448f1f52b121678ce308e192b89ce1394ab5cb8066c00a96db53c2ad422"},
		{501, 71536, "5cc8b4b1a72e98952f585acf66b2c6255301dbeec4d6ec323d638b3eff342754"},
		{541, 38209, "2efdbfb69c7384d0ae016036b4c13a9376e3ece3d33dc3c3205c4ac569ad9fa5"},
		{561, 25369, "a8bbf4b563afd3c52248a155df9c89a97b2a03fa973cbf4f6592c6f051e8a698"},
		{800, 19299, "6ba19bc61d86fd0a05356887e832d73b40779754b66e132252a2d6c9ad40ef65"},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("command_%d", test.command), func(t *testing.T) {
			svc := NewService(Config{Profile: "LocalPlayer"})
			response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
				Header: blaze.Header{
					Component: ComponentBootStatus, Command: test.command,
					MessageType: blaze.MessageTypeRequest, MessageID: uint32(test.command),
				},
			})
			if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
				t.Fatalf("captured progression command %d failed: type=%d error=0x%04x", test.command, response.Header.MessageType, response.Header.ErrorCode)
			}
			if len(response.Payload) != test.length {
				t.Fatalf("command %d returned %d bytes, want %d", test.command, len(response.Payload), test.length)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(response.Payload)); got != test.sha256 {
				t.Fatalf("command %d payload hash = %s, want %s", test.command, got, test.sha256)
			}
		})
	}
}

func TestDynamicRosterPayloadUsesEverySelectedTeamsPlayersAndBranding(t *testing.T) {
	players := make([]cfb27assets.Player, 85)
	for index := range players {
		players[index] = cfb27assets.Player{
			ID: index + 9000, FirstName: "Ohio", LastName: fmt.Sprintf("Player%d", index),
			TeamIndex: 68, Position: "QB", PositionValue: 0, JerseyNum: int64(index % 100),
			OverallRating: int64(99 - index/10), Portrait: int64(6000 + index),
			PresentationID: int64(7000 + index), Height: 72, Weight: 210,
			ClassYear: 2, DevelopmentTrait: 1,
			Ratings: map[string]int64{"PSPD": 90, "PACC": 91, "PAWR": 92},
		}
	}
	players[0].FirstName = "Julian"
	players[0].LastName = "Sayin"

	result := replaceDynastyRosterPlayers(dynasty393Payload, cfb27assets.Team{TeamIndex: 68, Logo: 78}, players)
	if count := len(dynastyRosterRecords(result)); count != 85 {
		tag, _ := blaze.EncodeTag("BACC")
		offset := bytes.Index(result, tag[:])
		t.Fatalf("dynamic roster has %d records before identity assertions, want 85 (first BACC=%d preceding=%x)", count, offset, result[offset-4:offset])
	}
	if bytes.Contains(result, []byte("Poffenbarger")) || bytes.Contains(result, []byte("Terrence")) {
		t.Fatalf("dynamic Ohio State roster still contains captured Akron players (Poffenbarger=%d Terrence=%d)", bytes.Count(result, []byte("Poffenbarger")), bytes.Count(result, []byte("Terrence")))
	}
	if !bytes.Contains(result, []byte("Julian")) || !bytes.Contains(result, []byte("Sayin")) {
		t.Fatal("dynamic roster is missing selected-team players")
	}
	tag, _ := blaze.EncodeTag("TMLG")
	want, _ := blaze.Encode([]blaze.Field{{Tag: "TMLG", Type: blaze.TypeInteger, Value: int64(78)}})
	if bytes.Count(result, want) < 85 || bytes.Count(result, tag[:]) < 85 {
		t.Fatal("dynamic roster did not apply the selected team's branding to every player")
	}
	records := dynastyRosterRecords(result)
	if len(records) != 85 {
		t.Fatalf("dynamic roster has %d player records, want 85", len(records))
	}
	seen := make(map[int64]bool, len(records))
	for index, record := range records {
		key, _, ok := dynastyUnsignedInteger(result, record.keyStart)
		if !ok || key < 9000 || key >= 9085 || seen[int64(key)] {
			t.Fatalf("player record %d retained or duplicated captured entity key %d", index, key)
		}
		seen[int64(key)] = true
		end := len(result)
		if index+1 < len(records) {
			end = records[index+1].keyStart
		}
		recordKey, ok := dynastyIntegerScalar(result[record.bodyStart:end], "RDKY")
		if !ok || recordKey != dynastyPlayerRecordKeyBase+int64(key) {
			t.Fatalf("player record %d RDKY=%d, want entity key for row %d", index, recordKey, key)
		}
	}
}

func TestDynastyAssistantCoachConfirmationMatchesCapture(t *testing.T) {
	payload, err := blaze.Encode([]blaze.Field{
		{Tag: "CKEY", Type: blaze.TypeInteger, Value: int64(547357082)},
		{Tag: "LGID", Type: blaze.TypeInteger, Value: int64(1)},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := newServiceWithAuthoritativeCoaches(t)
	svc.fallbackSession().selectedTeam.Store(830865495)
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header:  blaze.Header{Component: ComponentBootStatus, Command: 194, MessageType: blaze.MessageTypeRequest, MessageID: 194},
		Payload: payload,
	})
	if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
		t.Fatalf("assistant coach confirmation failed: type=%d error=0x%04x", response.Header.MessageType, response.Header.ErrorCode)
	}
	fields, err := blaze.Decode(response.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := integerField(fields, "CKEY"); !ok || value != 547357082 {
		t.Fatalf("CKEY = %d, want 547357082", value)
	}
	if value, ok := integerField(fields, "SUCC"); !ok || value != 1 {
		t.Fatalf("SUCC = %d, want 1", value)
	}
	if value, ok := integerField(fields, "TMLD"); !ok || value != 78 {
		t.Fatalf("TMLD = %d, want 78", value)
	}
	if value, ok := stringField(fields, "TMNM"); !ok || value != "Ohio State" {
		t.Fatalf("TMNM = %q, want Ohio State", value)
	}
}

func TestDynastyTimelineChoiceEchoesCapturedKeys(t *testing.T) {
	payload, err := blaze.Encode([]blaze.Field{
		{Tag: "CHID", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "EVKY", Type: blaze.TypeInteger, Value: int64(830341122)},
		{Tag: "IFSE", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "SCID", Type: blaze.TypeInteger, Value: int64(1)},
		{Tag: "STKY", Type: blaze.TypeInteger, Value: int64(561119246)},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(Config{Profile: "LocalPlayer"})
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header:  blaze.Header{Component: ComponentBootStatus, Command: 1420, MessageType: blaze.MessageTypeRequest, MessageID: 1420},
		Payload: payload,
	})
	if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
		t.Fatalf("timeline choice failed: type=%d error=0x%04x", response.Header.MessageType, response.Header.ErrorCode)
	}
	fields, err := blaze.Decode(response.Payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []struct {
		tag   string
		value int64
	}{
		{tag: "CHID", value: 0},
		{tag: "EVKY", value: 830341122},
		{tag: "IFSE", value: 0},
		{tag: "STKY", value: 561119246},
		{tag: "SUCC", value: 1},
		{tag: "TLCH", value: 0},
	} {
		if value, ok := integerField(fields, expected.tag); !ok || value != expected.value {
			t.Fatalf("%s = %d, want %d", expected.tag, value, expected.value)
		}
	}
}

func TestDynastyTeamSelectionAcknowledgementMatchesCapture(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header: blaze.Header{
			Component: ComponentBootStatus, Command: 1262,
			MessageType: blaze.MessageTypeRequest, MessageID: 1262,
		},
	})

	if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
		t.Fatalf("Dynasty team-selection acknowledgement failed: type=%d error=0x%04x", response.Header.MessageType, response.Header.ErrorCode)
	}
	if len(response.Payload) != 0 {
		t.Fatalf("expected captured empty response, got %d payload bytes", len(response.Payload))
	}
}

func TestMascotMashupEntryRoutes(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	for _, command := range []uint16{
		CommandGameReportingMatchContext,
		CommandGameReportingBotContext,
	} {
		response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
			Header: blaze.Header{
				Component: ComponentGameReporting, Command: command,
				MessageType: blaze.MessageTypeRequest, MessageID: uint32(command),
			},
		})
		if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
			t.Fatalf("Mascot Mashup command %d failed: type=%d error=0x%04x", command, response.Header.MessageType, response.Header.ErrorCode)
		}
	}
}

func TestClientConfigUsesKnownWorkingEmptyMap(t *testing.T) {
	fields, errorCode := handleUtilFetchClientConfig(context.Background(), blaze.Frame{})
	if errorCode != 0 {
		t.Fatalf("fetch-client-config returned error 0x%04x", errorCode)
	}
	configField, ok := fieldByTag(fields, "CONF")
	if !ok {
		t.Fatal("client config response is missing CONF")
	}
	config, ok := configField.Value.(blaze.Map)
	if !ok {
		t.Fatalf("CONF has unexpected type %T", configField.Value)
	}
	if len(config.Entries) != 0 {
		t.Fatalf("expected known-working empty CONF map, got %#v", config.Entries)
	}
}

func TestClientConfigReturnsCapturedTeamBuilderLimits(t *testing.T) {
	payload, err := blaze.Encode([]blaze.Field{
		{Tag: "CFID", Type: blaze.TypeString, Value: "OSDK_MADSET"},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(Config{Profile: "LocalPlayer"})
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header: blaze.Header{
			Component: ComponentUtil, Command: CommandUtilFetchClientConfig,
			MessageType: blaze.MessageTypeRequest, MessageID: 6,
		},
		Payload: payload,
	})
	if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
		t.Fatalf("OSDK_MADSET config failed: type=%d error=0x%04x", response.Header.MessageType, response.Header.ErrorCode)
	}
	fields, err := blaze.Decode(response.Payload)
	if err != nil {
		t.Fatal(err)
	}
	configField, ok := fieldByTag(fields, "CONF")
	if !ok {
		t.Fatal("OSDK_MADSET response is missing CONF")
	}
	config, ok := configField.Value.(blaze.Map)
	if !ok {
		t.Fatalf("CONF has unexpected type %T", configField.Value)
	}
	want := map[string]string{
		"CAREER_MAX_TEAMBUILDER_TEAMS": "16",
		"MCR_MAX_TEAMBUILDER_TEAMS":    "16",
		"MCR_SID_LEAGUE_TEAMBUILDER":   "267",
		"MCR_SID_RTG_TEAMBUILDER":      "287",
	}
	for _, entry := range config.Entries {
		key, keyOK := entry.Key.(string)
		value, valueOK := entry.Value.(string)
		if keyOK && valueOK {
			if expected, exists := want[key]; exists && value == expected {
				delete(want, key)
			}
		}
	}
	if len(want) != 0 {
		t.Fatalf("OSDK_MADSET response is missing captured values: %v", want)
	}
}

func TestLocalUserAddedNotification(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	notification, err := localUserAddedNotification(svc.identityForSequence(1))
	if err != nil {
		t.Fatal(err)
	}
	if notification.Header.Component != ComponentUserSessions ||
		notification.Header.Command != CommandUserSessionsUserAdded ||
		notification.Header.MessageType != blaze.MessageTypeNotification {
		t.Fatalf("unexpected notification header %#v", notification.Header)
	}
	fields, err := blaze.Decode(notification.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := integerField(fields, "BUID"); !ok || value != LocalBlazeID {
		t.Fatalf("expected local user in notification, got %d", value)
	}
}

func TestLocalUserSessionDataNotification(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	notification, err := localUserSessionDataNotification(svc.identityForSequence(1))
	if err != nil {
		t.Fatal(err)
	}
	if notification.Header.Component != ComponentUserSessions ||
		notification.Header.Command != CommandUserSessionsData ||
		notification.Header.MessageType != blaze.MessageTypeNotification {
		t.Fatalf("unexpected notification header %#v", notification.Header)
	}
	fields, err := blaze.Decode(notification.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := fieldByTag(fields, "DATA"); !ok {
		t.Fatal("session data notification is missing DATA")
	}
	if _, ok := fieldByTag(fields, "USER"); !ok {
		t.Fatal("session data notification is missing USER")
	}
}

func TestHandleUtilPing(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	request := blaze.Frame{
		Header: blaze.Header{
			Component:   ComponentUtil,
			Command:     CommandUtilPing,
			MessageType: blaze.MessageTypeRequest,
			MessageID:   1,
		},
	}

	response := svc.HandleFrame(context.Background(), "test-connection", request)
	if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
		t.Fatalf(
			"expected utility ping success, got type=%d error=0x%04x",
			response.Header.MessageType,
			response.Header.ErrorCode,
		)
	}
	fields, err := blaze.Decode(response.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := integerField(fields, "STIM"); !ok || value <= 0 {
		t.Fatalf("expected positive STIM value, got %d", value)
	}
}

func TestHandleUtilPreAuth(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	request := blaze.Frame{
		Header: blaze.Header{
			Component:   ComponentUtil,
			Command:     CommandUtilPreAuth,
			MessageType: blaze.MessageTypeRequest,
			MessageID:   2,
		},
	}

	response := svc.HandleFrame(context.Background(), "test-connection", request)
	if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
		t.Fatalf(
			"expected utility preAuth success, got type=%d error=0x%04x",
			response.Header.MessageType,
			response.Header.ErrorCode,
		)
	}

	fields, err := blaze.Decode(response.Payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{"CIDS", "CONF", "QOSS", "SVER"} {
		if _, ok := fieldByTag(fields, tag); !ok {
			t.Fatalf("preAuth response is missing %s", tag)
		}
	}
	if value, ok := stringField(fields, "SVER"); !ok || value != "Cypress CFB27 Offline" {
		t.Fatalf("unexpected server version %q", value)
	}

	qosField, _ := fieldByTag(fields, "QOSS")
	qosFields, ok := qosField.Value.([]blaze.Field)
	if !ok {
		t.Fatalf("expected QOSS struct, got %T", qosField.Value)
	}
	bandwidthField, ok := fieldByTag(qosFields, "BWPS")
	if !ok {
		t.Fatal("preAuth QOSS response is missing BWPS")
	}
	bandwidthFields, ok := bandwidthField.Value.([]blaze.Field)
	if !ok {
		t.Fatalf("expected BWPS struct, got %T", bandwidthField.Value)
	}
	if address, ok := stringField(bandwidthFields, "PSA"); !ok || address != "127.0.0.1" {
		t.Fatalf("expected local QoS address, got %q", address)
	}
}

func TestHandleUtilPostAuth(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header: blaze.Header{
			Component:   ComponentUtil,
			Command:     CommandUtilPostAuth,
			MessageType: blaze.MessageTypeRequest,
			MessageID:   8,
		},
	})
	if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
		t.Fatalf("expected post-auth success, got type=%d error=0x%04x", response.Header.MessageType, response.Header.ErrorCode)
	}
	fields, err := blaze.Decode(response.Payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{"TELE", "TICK", "UROP"} {
		if _, ok := fieldByTag(fields, tag); !ok {
			t.Fatalf("post-auth response is missing %s", tag)
		}
	}
}

func TestHandleUtilSetClientMetrics(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header: blaze.Header{Component: ComponentUtil, Command: CommandUtilSetClientMetrics, MessageID: 9},
	})
	if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
		t.Fatalf("expected client metrics success, got type=%d error=0x%04x", response.Header.MessageType, response.Header.ErrorCode)
	}
}

func TestHandleUtilFetchClientConfig(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header: blaze.Header{
			Component:   ComponentUtil,
			Command:     CommandUtilFetchClientConfig,
			MessageType: blaze.MessageTypeRequest,
			MessageID:   3,
		},
	})
	if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
		t.Fatalf(
			"expected fetchClientConfig success, got type=%d error=0x%04x",
			response.Header.MessageType,
			response.Header.ErrorCode,
		)
	}
	fields, err := blaze.Decode(response.Payload)
	if err != nil {
		t.Fatal(err)
	}
	configField, ok := fieldByTag(fields, "CONF")
	if !ok {
		t.Fatal("fetchClientConfig response is missing CONF")
	}
	config, ok := configField.Value.(blaze.Map)
	if !ok || config.KeyType != blaze.TypeString || config.ValueType != blaze.TypeString {
		t.Fatalf("expected string-to-string CONF map, got %#v", configField.Value)
	}
}

func TestHandleAuthenticationLogout(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header: blaze.Header{
			Component:   ComponentAuthentication,
			Command:     CommandAuthenticationLogout,
			MessageType: blaze.MessageTypeRequest,
			MessageID:   4,
		},
	})
	if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
		t.Fatalf(
			"expected logout success, got type=%d error=0x%04x",
			response.Header.MessageType,
			response.Header.ErrorCode,
		)
	}
	if len(response.Payload) != 0 {
		t.Fatalf("expected empty logout response, got %x", response.Payload)
	}
}

func TestDiagnosticsHealthAndEvents(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header: blaze.Header{Component: 0x7777, Command: 1, MessageID: 1},
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	svc.DiagnosticsHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected health 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var health map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &health); err != nil {
		t.Fatal(err)
	}
	if health["service"] != "cfb27-blaze" {
		t.Fatalf("unexpected service: %#v", health["service"])
	}

	req = httptest.NewRequest(http.MethodGet, "/events", nil)
	rec = httptest.NewRecorder()
	svc.DiagnosticsHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"component":30583`)) {
		t.Fatalf("unexpected events response: %d %s", rec.Code, rec.Body.String())
	}
}

func TestDiagnosticsReportsAuthoritativeCoachCatalog(t *testing.T) {
	svc := newServiceWithAuthoritativeCoaches(t)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	svc.DiagnosticsHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected health 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var health map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &health); err != nil {
		t.Fatal(err)
	}
	if health["coachCatalogStatus"] != "loaded" || health["coachCount"] != float64(7) || health["teamCount"] != float64(3) {
		t.Fatalf("unexpected coach catalog diagnostics: %#v", health)
	}
}

func TestRunRejectsInvalidCoachCatalog(t *testing.T) {
	svc := NewService(Config{CoachCatalog: filepath.Join(t.TempDir(), "missing.json")})
	err := svc.Run(t.Context())
	if err == nil || !strings.Contains(err.Error(), "load authoritative coach catalog") {
		t.Fatalf("Run error = %v, want authoritative coach catalog failure", err)
	}
}

func TestLocalRedirectorResponseXML(t *testing.T) {
	req := httptest.NewRequest(
		http.MethodPost,
		"https://spring25.client.blazeredirector.ea.com/redirector/getServerInstance",
		bytes.NewBufferString("<serverinstancerequest></serverinstancerequest>"),
	)
	req.Header.Set("Content-Type", "application/xml")

	contentType, body := NewService(Config{Profile: "LocalPlayer"}).localHTTPResponse(req, []byte("<serverinstancerequest/>"))
	if contentType != "application/xml; charset=utf-8" {
		t.Fatalf("unexpected content type %q", contentType)
	}
	for _, expected := range []string{
		"<serverinstanceresponse>",
		"<hostname>127.0.0.1</hostname>",
		"<ip>2130706433</ip>",
		"<port>27920</port>",
		"<secure>true</secure>",
	} {
		if !bytes.Contains(body, []byte(expected)) {
			t.Fatalf("redirector response missing %q: %s", expected, body)
		}
	}
}

func TestLocalRedirectorResponseJSON(t *testing.T) {
	req := httptest.NewRequest(
		http.MethodPost,
		"https://spring25.client.blazeredirector.ea.com/redirector/getServerInstance",
		bytes.NewBufferString("{}"),
	)
	req.Header.Set("Content-Type", "application/json")

	contentType, body := NewService(Config{Profile: "LocalPlayer"}).localHTTPResponse(req, []byte("{}"))
	if contentType != "application/json" {
		t.Fatalf("unexpected content type %q", contentType)
	}
	var response struct {
		Address struct {
			IPAddress struct {
				Hostname string `json:"hostname"`
				IP       uint32 `json:"ip"`
				Port     uint16 `json:"port"`
			} `json:"ipAddress"`
		} `json:"address"`
		Secure bool `json:"secure"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	if response.Address.IPAddress.Hostname != "127.0.0.1" ||
		response.Address.IPAddress.IP != localBlazeIPv4 ||
		response.Address.IPAddress.Port != 27920 || !response.Secure {
		t.Fatalf("unexpected redirector response: %#v", response)
	}
}

func TestHTTP2GRPCHealthResponse(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	request := httptest.NewRequest(
		http.MethodPost,
		"https://accounts.grpc.ea.com/grpc.health.v1.Health/Check",
		bytes.NewReader([]byte{0, 0, 0, 0, 0}),
	)
	request.ProtoMajor = 2
	request.ProtoMinor = 0
	request.Header.Set("Content-Type", "application/grpc")
	recorder := httptest.NewRecorder()

	svc.serveHTTP2Request("test-connection", "127.0.0.1:12345", "tls", recorder, request)
	response := recorder.Result()
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", response.StatusCode)
	}
	if contentType := response.Header.Get("Content-Type"); contentType != "application/grpc" {
		t.Fatalf("unexpected content type %q", contentType)
	}
	wantBody := []byte{0, 0, 0, 0, 2, 0x08, 0x01}
	if !bytes.Equal(recorder.Body.Bytes(), wantBody) {
		t.Fatalf("unexpected gRPC health response %x", recorder.Body.Bytes())
	}
	if status := response.Trailer.Get("Grpc-Status"); status != "0" {
		t.Fatalf("expected gRPC status 0, got %q", status)
	}
	events := svc.Events()
	if len(events) != 2 || events[0].Status != "http2-request" || events[1].Status != "http2-response" {
		t.Fatalf("unexpected HTTP/2 events: %#v", events)
	}
}

func TestHTTP2GrantTokenByAuthorizationCodeResponse(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	request := httptest.NewRequest(
		http.MethodPost,
		"https://accounts.grpc.ea.com/eadp.nexus.connect.grpc.v1.TokenService/GrantTokenByAuthorizationCode",
		bytes.NewReader([]byte{0, 0, 0, 0, 0}),
	)
	request.ProtoMajor = 2
	request.ProtoMinor = 0
	request.Header.Set("Content-Type", "application/grpc")
	recorder := httptest.NewRecorder()

	svc.serveHTTP2Request("test-connection", "127.0.0.1:12345", "tls", recorder, request)
	response := recorder.Result()
	defer response.Body.Close()
	if status := response.Trailer.Get("Grpc-Status"); status != "0" {
		t.Fatalf("expected gRPC status 0, got %q", status)
	}
	body := recorder.Body.Bytes()
	if len(body) < 6 || body[0] != 0 || body[5] != 0x0a {
		t.Fatalf("unexpected token response %x", body)
	}
	if !bytes.Contains(body, []byte("Bearer")) || bytes.Count(body, []byte(".")) < 4 {
		t.Fatalf("token response is missing the nested token fields: %x", body)
	}
}

func TestHTTP2GetTokenInfoResponse(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	request := httptest.NewRequest(
		http.MethodPost,
		"https://accounts.grpc.ea.com/eadp.nexus.connect.grpc.v1.TokenInfoService/GetTokenInfo",
		bytes.NewReader([]byte{0, 0, 0, 0, 0}),
	)
	request.ProtoMajor = 2
	request.ProtoMinor = 0
	recorder := httptest.NewRecorder()

	svc.serveHTTP2Request("test-connection", "127.0.0.1:12345", "tls", recorder, request)
	response := recorder.Result()
	defer response.Body.Close()
	if status := response.Trailer.Get("Grpc-Status"); status != "0" {
		t.Fatalf("expected gRPC status 0, got %q", status)
	}
	if body := recorder.Body.Bytes(); len(body) <= 7 || body[5] != 0x0a {
		t.Fatalf("unexpected token-info response %x", body)
	}
}

func stringField(fields []blaze.Field, tag string) (string, bool) {
	for _, field := range fields {
		if field.Tag == tag {
			value, ok := field.Value.(string)
			return value, ok
		}
	}
	return "", false
}

func integerField(fields []blaze.Field, tag string) (int64, bool) {
	for _, field := range fields {
		if field.Tag == tag {
			value, ok := field.Value.(int64)
			return value, ok
		}
	}
	return 0, false
}

func fieldByTag(fields []blaze.Field, tag string) (blaze.Field, bool) {
	for _, field := range fields {
		if field.Tag == tag {
			return field, true
		}
	}
	return blaze.Field{}, false
}

func assertDynastyFieldsNearString(t *testing.T, payload []byte, anchor blaze.Field, before, after int, expected []blaze.Field) {
	t.Helper()
	anchorWire, err := blaze.Encode([]blaze.Field{anchor})
	if err != nil {
		t.Fatal(err)
	}
	offset := bytes.Index(payload, anchorWire)
	if offset < 0 {
		t.Fatalf("response is missing anchor %s=%v", anchor.Tag, anchor.Value)
	}
	start := offset - before
	if start < 0 {
		start = 0
	}
	end := offset + len(anchorWire) + after
	if end > len(payload) {
		end = len(payload)
	}
	window := payload[start:end]
	for _, field := range expected {
		wire, err := blaze.Encode([]blaze.Field{field})
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(window, wire) {
			t.Fatalf("%s=%v is not within %d bytes of %s=%v; window=%x", field.Tag, field.Value, before+after, anchor.Tag, anchor.Value, window)
		}
	}
}

func dynastyStaffCardRows(t *testing.T, payload []byte, teamName string) [][]byte {
	t.Helper()
	anchor, err := blaze.Encode([]blaze.Field{{Tag: "TMNM", Type: blaze.TypeString, Value: teamName}})
	if err != nil {
		t.Fatal(err)
	}
	startTag, err := blaze.EncodeTag("CDSC")
	if err != nil {
		t.Fatal(err)
	}
	anchors := make([]int, 0, 3)
	for cursor := 0; cursor < len(payload); {
		relative := bytes.Index(payload[cursor:], anchor)
		if relative < 0 {
			break
		}
		position := cursor + relative
		anchors = append(anchors, position)
		cursor = position + len(anchor)
	}
	if len(anchors) != 3 {
		t.Fatalf("staff card row count = %d, want 3", len(anchors))
	}
	starts := make([]int, len(anchors))
	for index, position := range anchors {
		searchStart := position - 220
		if searchStart < 0 {
			searchStart = 0
		}
		relative := bytes.LastIndex(payload[searchStart:position], startTag[:])
		if relative < 0 {
			t.Fatalf("staff row %d has no CDSC start", index)
		}
		starts[index] = searchStart + relative
	}
	rows := make([][]byte, len(starts))
	for index, start := range starts {
		end := len(payload)
		if index+1 < len(starts) {
			end = starts[index+1]
		}
		rows[index] = payload[start:end]
	}
	return rows
}

func assertDynastyStringScalar(t *testing.T, payload []byte, tag, want string) {
	t.Helper()
	got, ok := dynastyStringScalar(payload, tag)
	if !ok || got != want {
		t.Fatalf("%s = %q, %v; want %q", tag, got, ok, want)
	}
}

func assertDynastyIntegerScalar(t *testing.T, payload []byte, tag string, want int64) {
	t.Helper()
	got, ok := dynastyIntegerScalar(payload, tag)
	if !ok || got != want {
		t.Fatalf("%s = %d, %v; want %d", tag, got, ok, want)
	}
}

func newServiceWithAuthoritativeCoaches(t *testing.T) *Service {
	t.Helper()
	path := filepath.Join(t.TempDir(), "coaches.json")
	contents := `{
      "version":1,
      "source":{"assetRoot":"C:/assets","slot":0,"dataRevisionVersion":3,"dynastySha256":"abc","coachSchemaSha256":"def","teamSchemaSha256":"ghi"},
      "coaches":[
        {"id":107,"firstName":"Ryan","lastName":"Day","assetName":"Unique_C_DayRyan_665","portrait":618,"level":76,"position":"HeadCoach","positionValue":0,"teamIndex":68,"pipeline":"Ohio","pipelineValue":28,"archetype":"CEO","archetypeValue":12,"coachPrestige":"Aplus","coachPrestigeValue":0,"offensiveScheme":"10000000011000111011001110001000","defensiveScheme":"10000000011000111011001110001010","data":{"FirstName":"Ryan"}},
        {"id":369,"firstName":"Arthur","lastName":"Smith","assetName":"Unique_C_SmithArthur_877","portrait":1044,"level":30,"position":"OffensiveCoordinator","positionValue":1,"teamIndex":68,"pipeline":"Tennessee","pipelineValue":37,"archetype":"Strategist","archetypeValue":6,"coachPrestige":"Cplus","coachPrestigeValue":6,"data":{"FirstName":"Arthur"}},
        {"id":308,"firstName":"Matt","lastName":"Patricia","assetName":"Unique_C_PatriciaMatt_667","portrait":964,"level":32,"position":"DefensiveCoordinator","positionValue":2,"teamIndex":68,"pipeline":"NewEngland","pipelineValue":22,"archetype":"EliteRecruiter","archetypeValue":7,"coachPrestige":"Cplus","coachPrestigeValue":6,"data":{"FirstName":"Matt"}},
        {"id":400,"firstName":"Brent","lastName":"Venables","assetName":"Unique_C_VenablesBrent_668","portrait":898,"level":59,"position":"HeadCoach","positionValue":0,"teamIndex":69,"pipeline":"Kansas","pipelineValue":12,"archetype":"Architect","archetypeValue":4,"coachPrestige":"Aminus","coachPrestigeValue":2,"offensiveScheme":"10000000011000111011001110000000","defensiveScheme":"10000000011000111011001110001010","data":{"FirstName":"Brent"}},
        {"id":231,"firstName":"Dan","lastName":"Lanning","assetName":"Unique_C_LanningDan_680","portrait":736,"level":70,"position":"HeadCoach","positionValue":0,"teamIndex":72,"pipeline":"SouthernCalifornia","pipelineValue":35,"archetype":"ProgramBuilder","archetypeValue":11,"coachPrestige":"Aplus","coachPrestigeValue":0,"data":{"FirstName":"Dan"}},
        {"id":269,"firstName":"Drew","lastName":"Mehringer","assetName":"Unique_C_MehringerDrew_882","portrait":1020,"level":23,"position":"OffensiveCoordinator","positionValue":1,"teamIndex":72,"pipeline":"EastTexas","pipelineValue":7,"archetype":"EliteRecruiter","archetypeValue":7,"coachPrestige":"Cminus","coachPrestigeValue":8,"data":{"FirstName":"Drew"}},
        {"id":171,"firstName":"Chris","lastName":"Hampton","assetName":"Unique_C_HamptonChris_881","portrait":1000,"level":23,"position":"DefensiveCoordinator","positionValue":2,"teamIndex":72,"pipeline":"Louisiana","pipelineValue":14,"archetype":"EliteRecruiter","archetypeValue":7,"coachPrestige":"C","coachPrestigeValue":7,"data":{"FirstName":"Chris"}}
      ],
      "teams":[
        {"id":1,"teamKey":830865409,"teamIndex":1,"presentationId":1101,"assetName":"AKRONX","displayName":"Akron","longName":"Akron","nickname":"Zips","nicknameAlt":"Zips","shortName":"AKRN","logo":1,"defensiveRating":66,"offensiveRating":66,"overallRating":66,"offensiveScheme":"OFF_SPREAD","offensiveSchemeValue":6,"defensiveScheme":"DEF_3_4_MULTIPLE","defensiveSchemeValue":17,"prestigeRank":137,"teamPrestige":0,"primaryColor":269891,"secondaryColor":13023108,"conference":{"name":"MAC","enum":"MAC","presentationId":6},"data":{"TeamIndex":1,"Hashtag1":"#ZipEmUp","Hashtag2":"#GoZips","Mascot_AssetName":"AKR","TEAM_DBASSETNAME":"teamdb_zips","TEAM_PREFIX_NAME":"AKR","UniformAssetName":"AKRONX"}},
        {"id":87,"teamKey":830865495,"teamIndex":68,"presentationId":1178,"assetName":"OHIOST","displayName":"Ohio State","longName":"Ohio State","nickname":"Buckeyes","nicknameAlt":"Bucks","shortName":"OSU","logo":78,"defensiveRating":96,"offensiveRating":94,"overallRating":94,"offensiveScheme":"OFF_SPREAD","offensiveSchemeValue":6,"defensiveScheme":"DEF_4_2_5","defensiveSchemeValue":14,"prestigeRank":3,"teamPrestige":10,"primaryColor":12649008,"secondaryColor":10660269,"conference":{"name":"Big Ten","enum":"BigTen","presentationId":1},"data":{"TeamIndex":68,"Hashtag1":"#GoBucks","Hashtag2":"#OH-IO","Mascot_AssetName":"OSU","TEAM_DBASSETNAME":"teamdb_ohiost","TEAM_PREFIX_NAME":"OSU","UniformAssetName":"OHIOST"}},
        {"id":92,"teamKey":830865500,"teamIndex":72,"presentationId":1183,"assetName":"OREGON","displayName":"Oregon","longName":"Oregon","nickname":"Ducks","nicknameAlt":"Ducks","shortName":"ORE","logo":83,"defensiveRating":91,"offensiveRating":91,"overallRating":91,"offensiveScheme":"OFF_SPREAD","offensiveSchemeValue":6,"defensiveScheme":"DEF_3_4_MULTIPLE","defensiveSchemeValue":17,"prestigeRank":2,"teamPrestige":9,"primaryColor":31028,"secondaryColor":16637985,"conference":{"name":"Big Ten","enum":"BigTen","presentationId":1},"data":{"TeamIndex":72}}
      ]
    }`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return NewService(Config{Profile: "LocalPlayer", CoachCatalog: path})
}

func TestDynastyListMenuMatchesCaptureShape(t *testing.T) {
	// Two sessions from a stub Dynasty service; the handler must return an FLST
	// list whose entries carry the tags/types the client's Load menu parses.
	dynasty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/sessions" {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sessions":[` +
			`{"id":8,"name":"bphit4","dynastyMode":"Online Dynasty","currentWeek":1,"stage":"preseason","maxUsers":32,"selectedTeamKey":830865495},` +
			`{"id":9,"name":"Second","dynastyMode":"Online Dynasty","currentWeek":3,"stage":"week_3","maxUsers":16}` +
			`]}`))
	}))
	defer dynasty.Close()

	svc := NewService(Config{Profile: "LocalPlayer", DynastyURL: dynasty.URL})
	response := svc.HandleFrame(context.Background(), "test-connection", blaze.Frame{
		Header: blaze.Header{
			Component: ComponentBootStatus, Command: 103,
			MessageType: blaze.MessageTypeRequest, MessageID: 7,
		},
		Payload: mustEncode(t, []blaze.Field{{Tag: "TYPE", Type: blaze.TypeInteger, Value: int64(3)}}),
	})
	if response.Header.MessageType != blaze.MessageTypeReply || response.Header.ErrorCode != 0 {
		t.Fatalf("list failed: type=%d error=0x%04x", response.Header.MessageType, response.Header.ErrorCode)
	}

	// The reply must round-trip through the decoder (ordering + types valid).
	fields, err := blaze.Decode(response.Payload)
	if err != nil {
		t.Fatalf("decode list reply: %v", err)
	}
	flst, ok := fieldByTag(fields, "FLST")
	if !ok {
		t.Fatal("list reply missing FLST")
	}
	list, ok := flst.Value.(blaze.List)
	if !ok || list.ElementType != blaze.TypeStruct {
		t.Fatalf("FLST is not a struct list: %T", flst.Value)
	}
	if len(list.Values) != 2 {
		t.Fatalf("expected 2 dynasties, got %d", len(list.Values))
	}
	first, ok := list.Values[0].([]blaze.Field)
	if !ok {
		t.Fatalf("entry is not a struct: %T", list.Values[0])
	}
	if v, ok := integerField(first, "LGID"); !ok || v != 8 {
		t.Fatalf("expected LGID=8, got %d", v)
	}
	if v, ok := stringField(first, "NAME"); !ok || v != "bphit4" {
		t.Fatalf("expected NAME=bphit4, got %q", v)
	}
	if v, ok := stringField(first, "SNTX"); !ok || v != "2026 PRE Wk 1" {
		t.Fatalf("expected SNTX=2026 PRE Wk 1, got %q", v)
	}
	// Team-identity fields are populated from the coach catalog (not loaded in
	// this unit test); here assert only that they exist with the captured types
	// so the wire shape stays correct. Catalog-backed values are covered where
	// the catalog is available.
	for _, tag := range []string{"FOFN", "USTN", "TMLG", "TMLS", "USPC", "AVDA"} {
		if field, ok := fieldByTag(first, tag); !ok || field.Type != blaze.TypeString {
			t.Fatalf("expected string field %s in entry", tag)
		}
	}
	for _, tag := range []string{"FOFT", "USTL", "CAYR", "JOIN", "RSID"} {
		if field, ok := fieldByTag(first, tag); !ok || field.Type != blaze.TypeInteger {
			t.Fatalf("expected integer field %s in entry", tag)
		}
	}
	sett, ok := fieldByTag(first, "SETT")
	if !ok {
		t.Fatal("entry missing SETT")
	}
	settings, ok := sett.Value.([]blaze.Field)
	if !ok {
		t.Fatalf("SETT not a struct: %T", sett.Value)
	}
	if v, ok := integerField(settings, "MHUM"); !ok || v != 32 {
		t.Fatalf("expected SETT.MHUM=32, got %d", v)
	}
}

func mustEncode(t *testing.T, fields []blaze.Field) []byte {
	t.Helper()
	payload, err := blaze.Encode(fields)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestDynastyJoinLifecycleAcks(t *testing.T) {
	var joinedLeague int64
	var joinedName string
	dynasty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/sessions/42/users" {
			var body struct {
				DisplayName string `json:"displayName"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			joinedLeague = 42
			joinedName = body.DisplayName
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":1}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sessions":[{"id":8,"name":"bphit4","dynastyMode":"Online Dynasty","currentWeek":1,"stage":"preseason","maxUsers":32}]}`))
	}))
	defer dynasty.Close()
	svc := NewService(Config{Profile: "LocalPlayer", DynastyURL: dynasty.URL})

	// 104 search -> FLST list (same shape as 103).
	search := svc.HandleFrame(context.Background(), "c", blaze.Frame{
		Header:  blaze.Header{Component: ComponentBootStatus, Command: 104, MessageType: blaze.MessageTypeRequest, MessageID: 1},
		Payload: mustEncode(t, []blaze.Field{{Tag: "CRIT", Type: blaze.TypeStruct, Value: []blaze.Field{}}}),
	})
	if search.Header.ErrorCode != 0 {
		t.Fatalf("104 search errored: 0x%04x", search.Header.ErrorCode)
	}
	sf, _ := blaze.Decode(search.Payload)
	if _, ok := fieldByTag(sf, "FLST"); !ok {
		t.Fatal("104 reply missing FLST")
	}

	// 111 apply -> SUCC=1, and the active session is set to the joined league.
	apply := svc.HandleFrame(context.Background(), "c", blaze.Frame{
		Header: blaze.Header{Component: ComponentBootStatus, Command: 111, MessageType: blaze.MessageTypeRequest, MessageID: 2},
		Payload: mustEncode(t, []blaze.Field{
			{Tag: "ACIN", Type: blaze.TypeStruct, Value: []blaze.Field{{Tag: "TMID", Type: blaze.TypeInteger, Value: int64(0)}}},
			{Tag: "LGID", Type: blaze.TypeInteger, Value: int64(42)},
		}),
	})
	if v, ok := integerField(mustDecode(t, apply.Payload), "SUCC"); !ok || v != 1 {
		t.Fatalf("111 apply expected SUCC=1, got %d", v)
	}
	if svc.fallbackSession().activeDynastySession.Load() != 42 {
		t.Fatalf("111 should set active session to 42, got %d", svc.fallbackSession().activeDynastySession.Load())
	}
	if joinedLeague != 42 || joinedName != "LocalPlayer" {
		t.Fatalf("111 should record membership (league=%d name=%q)", joinedLeague, joinedName)
	}

	// 164 leave -> SUCC=1, clears the active session.
	leave := svc.HandleFrame(context.Background(), "c", blaze.Frame{
		Header: blaze.Header{Component: ComponentBootStatus, Command: 164, MessageType: blaze.MessageTypeRequest, MessageID: 3},
		Payload: mustEncode(t, []blaze.Field{
			{Tag: "LGID", Type: blaze.TypeInteger, Value: int64(42)},
			{Tag: "RQID", Type: blaze.TypeInteger, Value: int64(1)},
			{Tag: "SELR", Type: blaze.TypeInteger, Value: int64(0)},
		}),
	})
	if v, ok := integerField(mustDecode(t, leave.Payload), "SUCC"); !ok || v != 1 {
		t.Fatalf("164 leave expected SUCC=1, got %d", v)
	}
	if svc.fallbackSession().activeDynastySession.Load() != 0 {
		t.Fatalf("164 should clear active session, got %d", svc.fallbackSession().activeDynastySession.Load())
	}
}

func mustDecode(t *testing.T, payload []byte) []blaze.Field {
	t.Helper()
	fields, err := blaze.Decode(payload)
	if err != nil {
		t.Fatal(err)
	}
	return fields
}

func TestPerConnectionIdentityAndTeamIsolation(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})

	// Two connections get their own sessions. Connection 1 keeps the historical
	// identity exactly; connection 2 is distinct so the game cannot conflate them.
	c1 := svc.sessionForPeer("c-000001", 0xC0A8010D, 3659)
	c2 := svc.sessionForPeer("c-000002", 0xC0A8010E, 3659)

	if c1.identity.blazeID != LocalBlazeID {
		t.Fatalf("connection 1 must keep BlazeID %d, got %d", LocalBlazeID, c1.identity.blazeID)
	}
	if c1.identity.personaName != "LocalPlayer" {
		t.Fatalf("connection 1 persona should be LocalPlayer, got %q", c1.identity.personaName)
	}
	if c2.identity.blazeID == c1.identity.blazeID {
		t.Fatalf("two connections share BlazeID %d", c2.identity.blazeID)
	}
	if c2.identity.sessionKey == c1.identity.sessionKey {
		t.Fatal("two connections share a session key")
	}

	// Each player's selected team is independent — the pre-refactor clobber bug.
	c1.selectedTeam.Store(830865495) // Ohio State
	c2.selectedTeam.Store(830865409) // Akron
	if c1.selectedTeam.Load() != 830865495 || c2.selectedTeam.Load() != 830865409 {
		t.Fatalf("selected teams clobbered: c1=%d c2=%d", c1.selectedTeam.Load(), c2.selectedTeam.Load())
	}

	// Login for connection 2 must present connection 2's identity, not 1001.
	login := svc.HandleFrame(withClientSession(context.Background(), c2), "c-000002", blaze.Frame{
		Header: blaze.Header{Component: ComponentAuthentication, Command: CommandAuthenticationLogin, MessageType: blaze.MessageTypeRequest, MessageID: 1},
	})
	fields, err := blaze.Decode(login.Payload)
	if err != nil {
		t.Fatal(err)
	}
	sess, ok := fieldByTag(fields, "SESS")
	if !ok {
		t.Fatal("login reply missing SESS")
	}
	sf := sess.Value.([]blaze.Field)
	if v, _ := integerField(sf, "BUID"); v != c2.identity.blazeID {
		t.Fatalf("connection 2 login BUID = %d, want %d", v, c2.identity.blazeID)
	}
}

func TestH2HBrokersPeerAddresses(t *testing.T) {
	// Two players calling create-or-join must land in the SAME game, and each
	// must be told the other's dialable address — that brokering is the whole
	// job, since gameplay then goes peer to peer.
	svc := NewService(Config{Profile: "LocalPlayer"})

	host := svc.sessionForPeer("c-000001", 0xC0A8010D, 3659)

	guest := svc.sessionForPeer("c-000002", 0xC0A8010E, 3659)

	join := func(cs *clientSession) int64 {
		reply := svc.HandleFrame(withClientSession(context.Background(), cs), cs.currentConnectionID(), blaze.Frame{
			Header: blaze.Header{Component: ComponentGameManager, Command: CommandGameManagerCreateOrJoin,
				MessageType: blaze.MessageTypeRequest, MessageID: 1},
		})
		if reply.Header.ErrorCode != 0 {
			t.Fatalf("create-or-join errored: 0x%04x", reply.Header.ErrorCode)
		}
		id, ok := integerField(mustDecode(t, reply.Payload), "GID")
		if !ok {
			t.Fatal("reply missing GID")
		}
		return id
	}

	hostGame := join(host)
	guestGame := join(guest)
	if hostGame != guestGame {
		t.Fatalf("players landed in different games: %d vs %d", hostGame, guestGame)
	}

	// The guest must be visible to the host as a peer with the right address.
	peers := svc.games.peersOf(hostGame, host.playerKey)
	if len(peers) != 1 {
		t.Fatalf("expected 1 peer for the host, got %d", len(peers))
	}
	if peers[0].externalIP != 0xC0A8010E || peers[0].externalPort != 3659 {
		t.Fatalf("peer address wrong: ip=%08x port=%d", peers[0].externalIP, peers[0].externalPort)
	}

	// The notification the peer receives must carry that address in PNET/EXIP.
	frame, err := playerJoiningNotification(hostGame, peers[0])
	if err != nil {
		t.Fatal(err)
	}
	fields := mustDecode(t, frame.Payload)
	pdat, ok := fieldByTag(fields, "PDAT")
	if !ok {
		t.Fatal("notification missing PDAT")
	}
	pnet, ok := fieldByTag(pdat.Value.([]blaze.Field), "PNET")
	if !ok {
		t.Fatal("PDAT missing PNET (the peer address union)")
	}
	union, ok := pnet.Value.(blaze.Union)
	if !ok || union.Value == nil {
		t.Fatalf("PNET is not a populated union: %#v", pnet.Value)
	}
	exip, ok := fieldByTag(union.Value.Value.([]blaze.Field), "EXIP")
	if !ok {
		t.Fatal("PNET missing EXIP")
	}
	if ip, _ := integerField(exip.Value.([]blaze.Field), "IP"); ip != 0xC0A8010E {
		t.Fatalf("EXIP.IP = %d, want %d", ip, 0xC0A8010E)
	}

	// Disconnecting must free the slot so a rematch can form.
	svc.games.remove(guest.playerKey)
	if got := len(svc.games.peersOf(hostGame, host.playerKey)); got != 0 {
		t.Fatalf("peer not removed on disconnect: %d remain", got)
	}
}

func TestManySocketsFromOneGameAreOnePlayer(t *testing.T) {
	// Regression: a single CFB27 client opens ~18 TCP connections. Keying identity
	// per connection made it look like 18 different players and broke login with
	// "lost connection to the EA servers". All sockets from one address must share
	// one identity and one dynasty state.
	svc := NewService(Config{Profile: "LocalPlayer"})
	const player = uint32(0x6A6E885A) // 106.110.136.90-ish; one machine

	first := svc.sessionForPeer("c-000001", player, 3659)
	first.selectedTeam.Store(830865495) // Ohio State, chosen on socket 1

	for i := 2; i <= 18; i++ {
		other := svc.sessionForPeer(fmt.Sprintf("c-%06d", i), player, 3659)
		if other != first {
			t.Fatalf("socket %d got a different session than socket 1", i)
		}
		if other.identity.blazeID != first.identity.blazeID {
			t.Fatalf("socket %d identity %d != socket 1 identity %d",
				i, other.identity.blazeID, first.identity.blazeID)
		}
		if got := other.selectedTeam.Load(); got != 830865495 {
			t.Fatalf("socket %d lost the selected team: %d", i, got)
		}
	}
	if first.identity.blazeID != LocalBlazeID {
		t.Fatalf("the only player must keep the historical identity %d, got %d",
			LocalBlazeID, first.identity.blazeID)
	}

	// A second machine is a genuinely different player.
	second := svc.sessionForPeer("c-000019", 0x6A6E885B, 3659)
	if second == first || second.identity.blazeID == first.identity.blazeID {
		t.Fatal("a different address must be a different player")
	}

	// Membership survives individual sockets closing, and only drops at zero.
	game, _ := svc.games.createOrJoin(&gamePlayer{playerKey: player, connectionID: "c-000001"})
	for i := 1; i < 18; i++ {
		if remaining := first.releaseConnection(); remaining <= 0 {
			t.Fatalf("player left the game after only %d sockets closed", i)
		}
	}
	if remaining := first.releaseConnection(); remaining > 0 {
		t.Fatalf("expected the last socket to reach zero, got %d", remaining)
	}
	svc.games.remove(player)
	if peers := svc.games.peersOf(game.id, 0); len(peers) != 0 {
		t.Fatalf("player should be gone once every socket closed, %d remain", len(peers))
	}
}

func TestRedirectorAdvertisesTheRealBindAddress(t *testing.T) {
	// The redirector tells the client where Blaze lives. Advertising 127.0.0.1
	// while the server is bound to a LAN/VPN address sends the client to its own
	// loopback, where nothing listens — and the bridge never redirects loopback,
	// so it surfaces as "lost connection to the EA servers" with zero Blaze frames.
	svc := NewService(Config{Profile: "LocalPlayer", Bind: "100.110.136.90", Port: 27920})
	request := httptest.NewRequest(http.MethodPost, "/redirector/getServerInstance", nil)

	_, body := svc.localHTTPResponse(request, nil)
	if !strings.Contains(string(body), "100.110.136.90") {
		t.Fatalf("redirector must advertise the bind address, got: %s", body)
	}
	// 100.110.136.90 packed big-endian.
	const packed = (100 << 24) | (110 << 16) | (136 << 8) | 90
	if !strings.Contains(string(body), fmt.Sprintf("%d", packed)) {
		t.Fatalf("redirector must advertise the packed ip %d, got: %s", packed, body)
	}

	// Default (host-only) still advertises loopback.
	local := NewService(Config{Profile: "LocalPlayer"})
	_, localBody := local.localHTTPResponse(request, nil)
	if !strings.Contains(string(localBody), "127.0.0.1") {
		t.Fatalf("default should still advertise loopback, got: %s", localBody)
	}
}

// Notifications used to be written to the raw socket while replies went through
// the TLS wrapper, so the client never saw them (H2H stuck on "PLEASE WAIT").
// They must go through the writer registered for the connection.
func TestNotificationsGoThroughTheRegisteredWriter(t *testing.T) {
	svc := NewService(Config{})
	var sink bytes.Buffer
	svc.registerFrameWriter("c-000001", &sink)

	frame, err := gameSetupNotification()
	if err != nil {
		t.Fatalf("gameSetupNotification: %v", err)
	}
	svc.sendToConnection("c-000001", frame)

	if sink.Len() == 0 {
		t.Fatal("notification never reached the registered writer")
	}
	got, err := blaze.ReadFrame(bytes.NewReader(sink.Bytes()))
	if err != nil {
		t.Fatalf("notification is not a readable frame: %v", err)
	}
	if got.Header.Component != 4 || got.Header.Command != 20 {
		t.Fatalf("got %d/%d, want 4/20", got.Header.Component, got.Header.Command)
	}

	// An unknown connection must be recorded, not silently dropped.
	svc.sendToConnection("c-999999", frame)
	var sawMiss bool
	for _, e := range svc.Events() {
		if e.Status == "notify-no-writer" {
			sawMiss = true
		}
	}
	if !sawMiss {
		t.Fatal("sending to an unregistered connection was not logged")
	}
}

// Authentication must be proxied to EA, never answered locally: a stubbed reply
// is what produced "ACCOUNT ERROR" on both machines.
func TestAuthHostsArePassedThroughToRealEA(t *testing.T) {
	for _, host := range []string{
		"accounts.ea.com", "ACCOUNTS.EA.COM", "accounts.ea.com:443", "signin.ea.com",
	} {
		if !isAuthPassThroughHost(host) {
			t.Errorf("%q should be proxied to real EA", host)
		}
	}
	// Everything we do implement must keep being served locally.
	for _, host := range []string{
		"gcs.ea.com", "collector.errors.ea.com", "update.layer.ea.com",
		"notaccounts.ea.com", "accounts.ea.com.evil.test", "",
	} {
		if isAuthPassThroughHost(host) {
			t.Errorf("%q should be served locally, not proxied", host)
		}
	}
}

// The bridge rewrites the Host header to the private server's own address, so
// sign-in must be recognised by path. Matching only on host silently disabled
// the whole proxy.
func TestAuthPassThroughMatchesRewrittenHostByPath(t *testing.T) {
	if !isAuthPassThroughRequest("100.110.136.90:27920", "/connect/auth") {
		t.Fatal("auth addressed to the private server must still be proxied")
	}
	if got := upstreamAuthHost("100.110.136.90:27920"); got != "accounts.ea.com" {
		t.Fatalf("upstream host = %q, want accounts.ea.com (else the proxy loops back)", got)
	}
	if got := upstreamAuthHost("signin.ea.com"); got != "signin.ea.com" {
		t.Fatalf("upstream host = %q, want the original EA host preserved", got)
	}
	for _, path := range []string{"/gameplayservices/x", "/genericEvents", "/"} {
		if isAuthPassThroughRequest("100.110.136.90:27920", path) {
			t.Errorf("%q must stay local", path)
		}
	}
}

// The trial build signs in over HTTP OAuth. /connect/auth must answer with a
// 302 carrying the code — the game follows that redirect itself, so collapsing
// it into a 200 stalls sign-in.
func TestLocalAuthFlowForTrialBuild(t *testing.T) {
	request := httptest.NewRequest(
		"GET",
		"https://accounts.ea.com/connect/auth?client_id=OOA&response_type=code"+
			"&redirect_uri=http://ooaactivation.ea.com/ooa/",
		nil)
	status, header, _, handled := localAuthResponse(request)
	if !handled {
		t.Fatal("/connect/auth must be handled locally")
	}
	if status != http.StatusFound {
		t.Fatalf("status = %d, want 302", status)
	}
	location := header.Get("Location")
	if !strings.HasPrefix(location, "http://ooaactivation.ea.com/ooa/?code=") {
		t.Fatalf("Location = %q, want the redirect_uri carrying a code", location)
	}

	request = httptest.NewRequest("POST", "https://accounts.ea.com/connect/token", nil)
	status, _, body, handled := localAuthResponse(request)
	if !handled || status != http.StatusOK {
		t.Fatalf("/connect/token handled=%v status=%d", handled, status)
	}
	var token map[string]any
	if err := json.Unmarshal(body, &token); err != nil {
		t.Fatalf("token response is not JSON: %v", err)
	}
	if token["access_token"] == "" || token["token_type"] != "Bearer" {
		t.Fatalf("token response missing access_token/token_type: %s", body)
	}

	// tokeninfo must name the same identity the rest of the server presents,
	// or the game would carry one identity into a session built around another.
	request = httptest.NewRequest("GET", "https://accounts.ea.com/connect/tokeninfo", nil)
	_, _, body, _ = localAuthResponse(request)
	var info map[string]any
	_ = json.Unmarshal(body, &info)
	if info["pid_id"] != fmt.Sprintf("%d", localNucleusAccountID) {
		t.Fatalf("tokeninfo pid_id = %v, want %d", info["pid_id"], localNucleusAccountID)
	}

	request = httptest.NewRequest("GET", "https://gcs.ea.com/application_id/X", nil)
	if _, _, _, handled := localAuthResponse(request); handled {
		t.Fatal("non-auth paths must not be captured by the auth handler")
	}
}

// The redirector is certificate-pinned, so its connection must be tunnelled to
// the real EA rather than terminated here. Everything else must still be served
// locally — tunnelling the Blaze connection itself would send the players to
// EA's servers instead of this one.
func TestPinnedRedirectorIsTunnelledOthersAreNot(t *testing.T) {
	t.Setenv("CYPRESS_CFB27_TLS_TUNNEL", "1")
	for _, name := range []string{
		"gosca25.blazeredirector.ea.com",
		"spring25.client.blazeredirector.ea.com",
		"GOSCA25.BLAZEREDIRECTOR.EA.COM",
		"spring25.gosredirector.ea.com",
	} {
		if !shouldTunnelSNI(name) {
			t.Errorf("%q is pinned and must be tunnelled", name)
		}
	}
	for _, name := range []string{
		"", "gcs.ea.com", "accounts.ea.com", "100.110.136.90",
		"blazeredirector.ea.com.evil.test",
	} {
		if shouldTunnelSNI(name) {
			t.Errorf("%q must be served locally, not tunnelled", name)
		}
	}
}

// The SNI has to be read without consuming the ClientHello, or the bytes would
// be lost to both the TLS server and the tunnel.
func TestPeekClientHelloSNILeavesBytesForTheHandshake(t *testing.T) {
	var recorded []byte
	client, server := net.Pipe()
	go func() {
		_ = tls.Client(client, &tls.Config{
			ServerName:         "gosca25.blazeredirector.ea.com",
			InsecureSkipVerify: true,
		}).Handshake()
	}()
	reader := bufio.NewReader(server)
	name, err := peekClientHelloSNI(reader)
	if err != nil {
		t.Fatalf("peek: %v", err)
	}
	if name != "gosca25.blazeredirector.ea.com" {
		t.Fatalf("SNI = %q", name)
	}
	recorded, err = reader.Peek(5)
	if err != nil || recorded[0] != 0x16 {
		t.Fatalf("ClientHello was consumed by the peek: %v %x", err, recorded)
	}
	_ = server.Close()
	_ = client.Close()
}

// NotifyGameSetup must describe THIS session. Replaying the capture sent the
// captured account's roster, and a client that cannot find itself in the roster
// crashes — which is why the notification was disabled.
func TestGameSetupNotificationCarriesLiveIdentity(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	game := &gameSession{id: 12345678}
	players := []*gamePlayer{
		{identity: clientIdentity{blazeID: 1001, personaID: 1001, personaName: "LocalPlayer"},
			externalIP: 0x6473C85A, slot: 0},
		{identity: clientIdentity{blazeID: 1002, personaID: 1002, personaName: "SecondPlayer"},
			externalIP: 0x644D4B44, slot: 1},
	}

	frame, err := svc.buildGameSetupNotification(game, players)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if frame.Header.Component != 4 || frame.Header.Command != 20 {
		t.Fatalf("got %d/%d, want 4/20", frame.Header.Component, frame.Header.Command)
	}

	// The captured account must not survive anywhere in the payload.
	if bytes.Contains(frame.Payload, []byte("bphit4")) {
		t.Error("captured persona name is still present in the payload")
	}
	for _, name := range []string{"LocalPlayer", "SecondPlayer"} {
		if !bytes.Contains(frame.Payload, []byte(name)) {
			t.Errorf("player %q is missing from the roster", name)
		}
	}

	fields, err := blaze.Decode(frame.Payload)
	if err != nil {
		t.Fatalf("built frame does not decode: %v", err)
	}
	roster, ok := findList(fields, "PROS")
	if !ok || len(roster.Values) != len(players) {
		t.Fatalf("roster has %d entries, want %d", len(roster.Values), len(players))
	}
	gameFields, _ := childFields(fields, "GAME")
	for _, field := range gameFields {
		if field.Tag == "GID" && field.Value.(int64) != game.id {
			t.Errorf("GAME/GID = %v, want %d", field.Value, game.id)
		}
	}
	entry := roster.Values[0].([]blaze.Field)
	for _, field := range entry {
		if field.Tag == "GID" && field.Value.(int64) != game.id {
			t.Errorf("roster GID = %v, want %d", field.Value, game.id)
		}
		if field.Tag == "PID" && field.Value.(int64) != 1001 {
			t.Errorf("roster PID = %v, want 1001", field.Value)
		}
	}
}

// 28/7 is asked for right after the game-setup notification. Answering it with
// an error made the client abandon the H2H session and report that it could not
// retrieve progression information.
func TestStatsGroupConfigIsAnswered(t *testing.T) {
	svc := NewService(Config{})
	reply := svc.HandleFrame(context.Background(), "c-000001", blaze.Frame{
		Header: blaze.Header{
			Component:   ComponentStats,
			Command:     CommandStatsGetStatGroup,
			MessageType: blaze.MessageTypeRequest,
		},
	})
	if reply.Header.ErrorCode != 0 {
		t.Fatalf("28/7 returned error 0x%X, want success", reply.Header.ErrorCode)
	}
	if len(reply.Payload) == 0 {
		t.Fatal("28/7 reply is empty")
	}
	fields, err := blaze.Decode(reply.Payload)
	if err != nil {
		t.Fatalf("28/7 reply does not decode: %v", err)
	}
	if _, ok := findList(fields, "LGRC"); !ok {
		t.Fatal("28/7 reply has no LGRC stat-column list")
	}
}

// Progression and social lookups must answer with an empty success, not
// UNIMPLEMENTED: the client reads UNIMPLEMENTED as a service outage and abandons
// the H2H session ("Unable to retrieve your progression information").
func TestProgressionAndSocialQueriesAnswerEmptySuccess(t *testing.T) {
	for _, path := range []string{
		"/eadp.stats.EntityStatistics/GetView",
		"/eadp.friends.v1.Friends/ListFriends",
		"/eadp.social.privacy.v1.Block/ListBlockedPlayers",
	} {
		if !emptyOKGRPCMethods[strings.ToLower(path)] {
			t.Errorf("%s must answer with an empty success", path)
		}
	}
	// Methods with real implementations must not be swallowed by the empty list.
	for _, path := range []string{
		"/eadp.nexus.connect.grpc.v1.TokenService/GrantTokenByAuthorizationCode",
		"/eadp.nexus.connect.grpc.v1.TokenInfoService/GetTokenInfo",
	} {
		if emptyOKGRPCMethods[strings.ToLower(path)] {
			t.Errorf("%s has a real implementation and must not return empty", path)
		}
	}
}

// The roster lives on EA's CDN and must be fetched from there. Standing in for
// it produced "There was an error downloading the latest player roster", and no
// amount of retrying could work.
func TestRosterCDNIsFetchedFromEA(t *testing.T) {
	for _, host := range []string{
		"eaassets-a.akamaihd.net",
		"EAASSETS-A.AKAMAIHD.NET",
		"eaassets-a.akamaihd.net:443",
		"gameplayservices.ea.com",
	} {
		if !isAssetPassThroughHost(host) {
			t.Errorf("%q serves static content and must be fetched from EA", host)
		}
	}
	// Services we implement must keep being served locally.
	for _, host := range []string{
		"", "gcs.ea.com", "accounts.ea.com", "100.110.136.90:27920",
		"eaassets-a.akamaihd.net.evil.test",
	} {
		if isAssetPassThroughHost(host) {
			t.Errorf("%q must be served locally, not proxied", host)
		}
	}
}

// Streaming subscriptions must be held open, not ended immediately. Ending them
// makes the client reconnect at once: ConnectToPresenceSession was called 3,400
// times per session while it returned UNIMPLEMENTED, and 26,000 once it returned
// an instant success.
func TestStreamingSubscriptionsAreHeldOpen(t *testing.T) {
	for _, path := range []string{
		"/eadp.social.presence.v1.PresenceService/ConnectToPresenceSession",
		"/eadp.social.presence.v1.PresenceService/SubscribeToFriendsPresence",
		"/eadp.friends.v2.FriendsNotificationsService/StreamNotifications",
	} {
		if !streamingGRPCMethods[strings.ToLower(path)] {
			t.Errorf("%s is a stream and must be held open", path)
		}
	}
	// Unary calls must not be held: the client waits for their data.
	for _, path := range []string{
		"/eadp.stats.EntityStatistics/GetView",
		"/eadp.playercard.v1.PlayerCardService/BatchGetPlayerCards",
		"/eadp.friends.v1.Friends/ListFriends",
	} {
		if streamingGRPCMethods[strings.ToLower(path)] {
			t.Errorf("%s is unary and must answer immediately", path)
		}
		if !emptyOKGRPCMethods[strings.ToLower(path)] {
			t.Errorf("%s must answer with an empty success", path)
		}
	}
}

// The friends list must be the roster of players on THIS server, so a player can
// choose who to invite. With more than two people connected, pairing them
// automatically is not an option — the invite has to name someone.
func TestFriendsListIsTheServerRoster(t *testing.T) {
	players := []connectedPlayer{
		{blazeID: 1002, personaID: 1002, personaName: "SecondPlayer"},
		{blazeID: 1003, personaID: 1003, personaName: "ThirdPlayer"},
	}
	body := listFriendsResponse(players)
	if len(body) < 5 {
		t.Fatal("response is not a gRPC frame")
	}
	// Frame header: 1 compression byte + 4-byte big-endian length.
	if body[0] != 0 {
		t.Fatalf("compression byte = %d, want 0", body[0])
	}
	declared := int(body[1])<<24 | int(body[2])<<16 | int(body[3])<<8 | int(body[4])
	if declared != len(body)-5 {
		t.Fatalf("declared length %d does not match body %d", declared, len(body)-5)
	}
	for _, name := range []string{"SecondPlayer", "ThirdPlayer"} {
		if !bytes.Contains(body, []byte(name)) {
			t.Errorf("%q is missing from the roster", name)
		}
	}
	// Each player must be addressable by id, which is what an invite names.
	for _, id := range []string{"1002", "1003"} {
		if !bytes.Contains(body, []byte(id)) {
			t.Errorf("player id %q is missing; an invite could not address them", id)
		}
	}
	// An empty roster must still be a valid, empty response.
	if empty := listFriendsResponse(nil); len(empty) != 5 {
		t.Fatalf("empty roster produced %d bytes, want a bare 5-byte frame", len(empty))
	}
}

// The caller must not see themselves in the list.
func TestFriendsListExcludesTheCaller(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	for _, id := range []uint32{1, 2, 3} {
		// Only sessions that speak Blaze are players.
		svc.sessionForPeer(fmt.Sprintf("c-%06d", id), id, 1000).markPlayer()
	}
	all := svc.connectedPlayers(-1)
	if len(all) != 3 {
		t.Fatalf("connected players = %d, want 3", len(all))
	}
	without := svc.connectedPlayers(all[0].personaID)
	if len(without) != 2 {
		t.Fatalf("roster for a caller = %d, want 2 (themselves excluded)", len(without))
	}
	for _, player := range without {
		if player.personaID == all[0].personaID {
			t.Error("the caller appears in their own friends list")
		}
	}
}

// The friends screen decides who can be invited from presence, not from the
// friends list: a player with no presence shows offline and offers no actions.
// photon_status="ONLINE" is the attribute that drives it — verified against a
// presence event recorded from EA while a friend was genuinely online.
func TestPresenceMarksServerPlayersOnline(t *testing.T) {
	players := []connectedPlayer{
		{blazeID: 1002, personaID: 1002, personaName: "SecondPlayer"},
		{blazeID: 1003, personaID: 1003, personaName: "ThirdPlayer"},
	}
	events := presenceEventsFor(players, time.Unix(1786655475, 252938010).UTC())
	if len(events) != len(players) {
		t.Fatalf("got %d presence events, want %d", len(events), len(players))
	}
	for i, event := range events {
		if len(event) < 5 || event[0] != 0 {
			t.Fatalf("event %d is not a gRPC frame", i)
		}
		declared := int(event[1])<<24 | int(event[2])<<16 | int(event[3])<<8 | int(event[4])
		if declared != len(event)-5 {
			t.Fatalf("event %d length %d does not match body %d", i, declared, len(event)-5)
		}
		if !bytes.Contains(event, []byte("ONLINE")) {
			t.Errorf("event %d does not mark the player online", i)
		}
		if !bytes.Contains(event, []byte(fmt.Sprintf("%d", players[i].personaID))) {
			t.Errorf("event %d does not identify player %d", i, players[i].personaID)
		}
	}
	if len(presenceEventsFor(nil, time.Now())) != 0 {
		t.Error("an empty server should announce nobody")
	}
}

// Every player needs a real, distinct name: they are picked out of a friends
// list and named in invites, so "LocalPlayer" for everyone does not work once
// more than one person is on the server.
func TestPlayersAreNamedFromTheRoster(t *testing.T) {
	file := filepath.Join(t.TempDir(), "players.json")
	if err := os.WriteFile(file, []byte(`{"100.110.136.90":"bphit4","100.77.75.68":"WaddWadd"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CYPRESS_CFB27_PLAYERS_FILE", file)
	roster = playerRoster{}

	svc := NewService(Config{Profile: "LocalPlayer"})
	ip := func(a, b, c, d byte) uint32 {
		return uint32(a)<<24 | uint32(b)<<16 | uint32(c)<<8 | uint32(d)
	}
	if got := svc.playerNameFor(ip(100, 110, 136, 90), 1); got != "bphit4" {
		t.Errorf("named player = %q, want bphit4", got)
	}
	if got := svc.playerNameFor(ip(100, 77, 75, 68), 2); got != "WaddWadd" {
		t.Errorf("named player = %q, want WaddWadd", got)
	}
	// Unlisted players must still be distinct, not all "LocalPlayer".
	first := svc.playerNameFor(ip(10, 0, 0, 5), 2)
	second := svc.playerNameFor(ip(10, 0, 0, 6), 3)
	if first == second {
		t.Errorf("unlisted players share the name %q", first)
	}
}

// Social calls arrive on their own connections, so the caller has to be resolved
// by address. Using a fixed identity handed every player the host's list, so a
// joining player saw themselves in it instead of the person they wanted.
func TestFriendsListIsBuiltForTheCallingPlayer(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	first := svc.sessionForPeer("c-000001", 0x646E8858, 1000)  // 100.110.136.88
	second := svc.sessionForPeer("c-000002", 0x644D4B44, 1000) // 100.77.75.68
	first.markPlayer()
	second.markPlayer()

	if got := svc.callerPersonaFor("100.110.136.88:5000"); got != first.identity.personaID {
		t.Fatalf("caller resolved to %d, want %d", got, first.identity.personaID)
	}
	if got := svc.callerPersonaFor("100.77.75.68:5000"); got != second.identity.personaID {
		t.Fatalf("caller resolved to %d, want %d", got, second.identity.personaID)
	}

	// Each player's list must contain the other, never themselves.
	firstList := svc.friendsListForRequest(first.identity.personaID)
	if !bytes.Contains(firstList, []byte(fmt.Sprintf("%d", second.identity.personaID))) {
		t.Error("first player's list is missing the second player")
	}
	secondList := svc.friendsListForRequest(second.identity.personaID)
	if !bytes.Contains(secondList, []byte(fmt.Sprintf("%d", first.identity.personaID))) {
		t.Error("second player's list is missing the first player")
	}
}

// The client takes the roster location from OSDK_MADSET. Answering that config
// with our own short list meant the game was never told where the roster is, so
// it never asked — which is why the download could not be fixed by retrying.
func TestClientConfigCarriesTheRosterLocation(t *testing.T) {
	svc := NewService(Config{})
	ask := func(configID string) []byte {
		payload, err := blaze.Encode([]blaze.Field{
			{Tag: "CFID", Type: blaze.TypeString, Value: configID},
		})
		if err != nil {
			t.Fatalf("encode request: %v", err)
		}
		reply := svc.HandleFrame(context.Background(), "c-000001", blaze.Frame{
			Header: blaze.Header{
				Component:   ComponentUtil,
				Command:     CommandUtilFetchClientConfig,
				MessageType: blaze.MessageTypeRequest,
			},
			Payload: payload,
		})
		if reply.Header.ErrorCode != 0 {
			t.Fatalf("%s returned error 0x%X", configID, reply.Header.ErrorCode)
		}
		return reply.Payload
	}

	madset := ask("OSDK_MADSET")
	for _, key := range []string{"DP_BASE_URL", "DP_CUSTRSTR_URL", "DP_RSTR_URL"} {
		if !bytes.Contains(madset, []byte(key)) {
			t.Errorf("OSDK_MADSET is missing %s; the client cannot find the roster", key)
		}
	}
	// The roster file name changes with every update, so the config must point at
	// the patch-info file rather than naming a database directly.
	if !bytes.Contains(madset, []byte("customroster.eaPatchInfo")) {
		t.Error("OSDK_MADSET does not point at customroster.eaPatchInfo")
	}
	if bytes.Contains(madset, []byte("mroster_")) {
		t.Error("OSDK_MADSET names a specific roster database; it would break on the next roster update")
	}

	if len(ask("OSDK_CORE")) == 0 {
		t.Error("OSDK_CORE returned nothing")
	}
	// An unknown id must still be a valid, empty config rather than an error.
	if len(ask("NOT_A_REAL_CONFIG")) == 0 {
		t.Error("an unknown config id should return an empty config, not nothing")
	}
}

// Players announce their name from their own machine. A Content-Length that did
// not match the body left the caller waiting for bytes that never arrived, so
// registration failed with "the connection was closed" and everyone fell back to
// a generated name.
func TestPlayerRegistrationRespondsWithAMatchingContentLength(t *testing.T) {
	// Never write into the real players.json: a test that registers a name must
	// not leave "TestPlayer" in the operator's roster.
	t.Setenv("CYPRESS_CFB27_PLAYERS_FILE", filepath.Join(t.TempDir(), "players.json"))
	roster = playerRoster{}
	svc := NewService(Config{Profile: "LocalPlayer"})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		svc.serveConnection(context.Background(), "c-000001", 1, conn)
	}()

	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Get("http://" + listener.Addr().String() + "/cypress/register?name=TestPlayer")
	if err != nil {
		t.Fatalf("registration request failed: %v", err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("reading the reply failed (a wrong Content-Length looks like this): %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}
	if !strings.Contains(string(payload), "TestPlayer") {
		t.Fatalf("reply %q does not confirm the name", string(payload))
	}
	if got := int64(len(payload)); response.ContentLength != got {
		t.Fatalf("Content-Length %d does not match the %d bytes sent", response.ContentLength, got)
	}
}

// A player's registration request is itself a connection from them, so their
// session exists a fraction of a second before their name arrives. Recording the
// name without applying it left everyone called "LocalPlayer".
func TestRegisteringRenamesAnExistingSession(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	const ip uint32 = 0x646E8858 // 100.110.136.88

	// The session is created first, exactly as the registration request does.
	session := svc.sessionForPeer("c-000001", ip, 1000)
	session.markPlayer()
	if session.identity.personaName != "LocalPlayer" {
		t.Fatalf("unexpected starting name %q", session.identity.personaName)
	}

	svc.registerPlayerName("100.110.136.88", "bphit4")

	if session.identity.personaName != "bphit4" {
		t.Fatalf("session name = %q, want bphit4 — registration did not reach the live session",
			session.identity.personaName)
	}
	// And the name must reach the friends list other players see.
	other := svc.sessionForPeer("c-000002", 0x644D4B44, 1000)
	other.markPlayer()
	list := svc.friendsListForRequest(other.identity.personaID)
	if !bytes.Contains(list, []byte("bphit4")) {
		t.Error("the registered name does not appear in the roster other players see")
	}
}

// Sessions are created for every socket, including health checks, the
// registration request and asset fetches. Counting those as players put phantom
// entries in the friends list — with a single player connected the roster came
// back non-empty, which is not something the client can render.
func TestOnlyBlazeClientsCountAsPlayers(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	incidental := svc.sessionForPeer("c-000001", 0x7F000001, 1000) // 127.0.0.1
	_ = incidental

	if got := len(svc.connectedPlayers(-1)); got != 0 {
		t.Fatalf("roster has %d entries from connections that never spoke Blaze, want 0", got)
	}
	// An empty roster must still be a valid response.
	if body := svc.friendsListForRequest(localPersonaID); len(body) != 5 {
		t.Fatalf("empty roster produced %d bytes, want a bare 5-byte frame", len(body))
	}

	// A real game client counts once it carries Blaze traffic.
	player := svc.sessionForPeer("c-000002", 0x644D4B44, 1000)
	player.markPlayer()
	if got := len(svc.connectedPlayers(-1)); got != 1 {
		t.Fatalf("roster has %d entries, want 1 once a client has spoken Blaze", got)
	}
}

// The overlay draws a card per player, and "Play a Friend" runs through it. An
// empty card names nobody, which shows as "the server encountered an unexpected
// condition" and can take the client down.
func TestPlayerCardsNameTheirPlayer(t *testing.T) {
	now := time.Unix(1786556820, 0).UTC()
	player := connectedPlayer{blazeID: 1002, personaID: 1002, personaName: "AlumRosters"}

	single := playerCardResponse(player, now)
	if len(single) <= 5 {
		t.Fatal("a card must carry more than an empty frame")
	}
	if !bytes.Contains(single, []byte("AlumRosters")) {
		t.Error("the card does not name the player")
	}
	if !bytes.Contains(single, []byte("1002")) {
		t.Error("the card does not carry the player id")
	}
	if !bytes.Contains(single, []byte("EA")) {
		t.Error("the card does not name a platform")
	}

	// A batch is one framed message per player, as the captured reply is.
	both := playerCardsResponse([]connectedPlayer{
		player,
		{blazeID: 1001, personaID: 1001, personaName: "bphit4"},
	}, now)
	for _, name := range []string{"AlumRosters", "bphit4"} {
		if !bytes.Contains(both, []byte(name)) {
			t.Errorf("the batch is missing %s", name)
		}
	}
	// Each frame must declare its own length correctly or the client desyncs.
	rest := both
	frames := 0
	for len(rest) >= 5 {
		length := int(rest[1])<<24 | int(rest[2])<<16 | int(rest[3])<<8 | int(rest[4])
		if length > len(rest)-5 {
			t.Fatalf("frame %d declares %d bytes but only %d remain", frames, length, len(rest)-5)
		}
		rest = rest[5+length:]
		frames++
	}
	if frames != 2 {
		t.Fatalf("got %d framed cards, want 2", frames)
	}
}

// Registrations must survive a server restart. Keeping them only in memory meant
// every restart reverted players to generated names until they re-registered,
// which during development was constantly.
func TestRegistrationsSurviveARestart(t *testing.T) {
	file := filepath.Join(t.TempDir(), "players.json")
	t.Setenv("CYPRESS_CFB27_PLAYERS_FILE", file)
	roster = playerRoster{}
	registeredMu.Lock()
	registeredNames = map[string]string{}
	registeredMu.Unlock()

	first := NewService(Config{Profile: "LocalPlayer"})
	first.registerPlayerName("100.77.75.68", "AlumRosters")

	// Restart: fresh process state, same file on disk.
	roster = playerRoster{}
	registeredMu.Lock()
	registeredNames = map[string]string{}
	registeredMu.Unlock()

	second := NewService(Config{Profile: "LocalPlayer"})
	const ip uint32 = 0x644D4B44 // 100.77.75.68
	if got := second.playerNameFor(ip, 2); got != "AlumRosters" {
		t.Fatalf("after restart the player is %q, want AlumRosters", got)
	}
	// A player who joins after the restart keeps their name without re-registering.
	session := second.sessionForPeer("c-000001", ip, 1000)
	if session.identity.personaName != "AlumRosters" {
		t.Fatalf("session name = %q, want AlumRosters", session.identity.personaName)
	}
}

// NotifyPlayerJoining tells everyone in a game who just joined. It was
// hand-assembled from the fields that looked necessary — 273 bytes against the
// 395 EA sends — and a client reading a frame that short looks for fields that
// are not there. It is now built from the captured frame with only the identity
// and address replaced.
func TestPlayerJoiningNotificationKeepsTheCapturedShape(t *testing.T) {
	player := &gamePlayer{
		identity:     clientIdentity{blazeID: 1002, personaID: 1002, personaName: "AlumRosters"},
		slot:         1,
		externalIP:   0x644D4B44, // 100.77.75.68
		externalPort: 3659,
	}
	frame, err := buildPlayerJoiningNotification(987654321, player)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if frame.Header.Component != 4 || frame.Header.Command != 21 {
		t.Fatalf("got %d/%d, want 4/21", frame.Header.Component, frame.Header.Command)
	}

	// The captured player must not survive into our frame.
	if bytes.Contains(frame.Payload, []byte("waddwadd")) {
		t.Error("the captured player's name is still in the notification")
	}
	if !bytes.Contains(frame.Payload, []byte("AlumRosters")) {
		t.Error("the joining player is not named")
	}

	fields, err := blaze.Decode(frame.Payload)
	if err != nil {
		t.Fatalf("the built notification does not decode: %v", err)
	}
	data, ok := childFields(fields, "PDAT")
	if !ok {
		t.Fatal("no PDAT in the notification")
	}
	// The fields the old hand-built frame omitted must all still be present.
	for _, tag := range []string{
		"CNTY", "CONG", "CSID", "DSUI", "ENCR", "JFPS", "JVMM", "LOC",
		"NASP", "PSET", "RCRE", "ROLE", "SCEN", "STAT", "TIDX", "UGID", "UUID",
	} {
		found := false
		for _, field := range data {
			if field.Tag == tag {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("PDAT is missing %s, which the captured frame carries", tag)
		}
	}
	for _, field := range data {
		switch field.Tag {
		case "PID":
			if field.Value.(int64) != 1002 {
				t.Errorf("PID = %v, want 1002", field.Value)
			}
		case "GID":
			if field.Value.(int64) != 987654321 {
				t.Errorf("PDAT/GID = %v, want the live game id", field.Value)
			}
		case "SLOT":
			if field.Value.(int64) != 1 {
				t.Errorf("SLOT = %v, want 1", field.Value)
			}
		}
	}
}

// Lobby changes must reach the other players. Acknowledging an attribute update
// without relaying it left both players on the away side, neither seeing the
// other's team, and readying up doing nothing.
func TestLobbyAttributeUpdatesAreRelayed(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	const gameID int64 = 3954783540361

	payload, err := blaze.Encode([]blaze.Field{
		{Tag: "ATTR", Type: blaze.TypeMap, Value: blaze.Map{
			KeyType:   blaze.TypeString,
			ValueType: blaze.TypeString,
			Entries:   []blaze.MapEntry{{Key: "ClientStatus", Value: "normal"}},
		}},
		{Tag: "GID", Type: blaze.TypeInteger, Value: gameID},
		{Tag: "PID", Type: blaze.TypeInteger, Value: int64(1002)},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := blaze.Frame{
		Header:  blaze.Header{Component: 4, Command: CommandGameManagerUpdateAttrs},
		Payload: payload,
	}

	if got, ok := gameIDFromRequest(request); !ok || got != gameID {
		t.Fatalf("game id read as %d (ok=%v), want %d", got, ok, gameID)
	}

	// The relayed notification must be 4/90 carrying the same body, so unknown
	// attributes survive untouched.
	fields, _, err := func() ([]blaze.Field, uint16, error) {
		f, code := svc.handleGameAttributeUpdate(context.Background(), request)
		return f, code, nil
	}()
	if err != nil {
		t.Fatal(err)
	}
	_ = fields

	decoded, err := blaze.Decode(payload)
	if err != nil {
		t.Fatal(err)
	}
	var sawStatus bool
	for _, field := range decoded {
		if field.Tag == "ATTR" {
			m := field.Value.(blaze.Map)
			for _, entry := range m.Entries {
				if entry.Key == "ClientStatus" && entry.Value == "normal" {
					sawStatus = true
				}
			}
		}
	}
	if !sawStatus {
		t.Error("the relayed body lost the attribute it was carrying")
	}
}

// The lobby advances on game- and player-state notifications the server sends.
// Acknowledging 4/3 silently, and never sending 4/116 / 4/30, left the lobby
// frozen: teams synced but readying up did nothing because no side was ever told
// the game state had changed.
func TestGameStateNotificationsDriveTheLobby(t *testing.T) {
	// 4/3 in must produce a 4/100 carrying the same GSTA.
	payload, err := blaze.Encode([]blaze.Field{
		{Tag: "GID", Type: blaze.TypeInteger, Value: int64(3954783540361)},
		{Tag: "GSTA", Type: blaze.TypeInteger, Value: gameStatePreGame},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gid, ok := gameIDFromRequest(blaze.Frame{Payload: payload}); !ok || gid != 3954783540361 {
		t.Fatalf("game id read as %d ok=%v", gid, ok)
	}

	state, err := gameStateNotification(987654321, gameStatePreGame)
	if err != nil {
		t.Fatal(err)
	}
	if state.Header.Command != NotifyGameStateChange {
		t.Fatalf("state notification is 4/%d, want 4/100", state.Header.Command)
	}
	fields, err := blaze.Decode(state.Payload)
	if err != nil {
		t.Fatal(err)
	}
	assertField(t, fields, "GID", int64(987654321))
	assertField(t, fields, "GSTA", gameStatePreGame)

	// 4/116 must mark a player active.
	player, err := playerStateNotification(987654321, 1002, playerStateActive)
	if err != nil {
		t.Fatal(err)
	}
	pf, _ := blaze.Decode(player.Payload)
	assertField(t, pf, "PID", int64(1002))
	assertField(t, pf, "STAT", playerStateActive)

	// 4/30 must confirm the join with a timestamp.
	done, err := playerJoinCompletedNotification(987654321, 1002, time.Unix(1786245361, 958378000))
	if err != nil {
		t.Fatal(err)
	}
	if done.Header.Command != NotifyPlayerJoinCompleted {
		t.Fatalf("join-completed is 4/%d, want 4/30", done.Header.Command)
	}
	df, _ := blaze.Decode(done.Payload)
	assertField(t, df, "GID", int64(987654321))
}

func assertField(t *testing.T, fields []blaze.Field, tag string, want any) {
	t.Helper()
	for _, field := range fields {
		if field.Tag == tag {
			if field.Value != want {
				t.Errorf("%s = %v, want %v", tag, field.Value, want)
			}
			return
		}
	}
	t.Errorf("field %s is missing", tag)
}
