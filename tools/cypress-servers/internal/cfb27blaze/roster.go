package cfb27blaze

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

/*
Who each player is on this server.

After Blaze login the client displays the identity the SERVER gave it, which is
why the name flips from the EA account to ours at the main menu. The Blaze login
carries only an encrypted token bundle, so the client cannot tell us its EA
persona — the server has to decide.

With one player "LocalPlayer" was fine. For a multi-user dynasty it is not:
players pick each other out of a friends list and invites name a specific
person, so everyone needs a real, stable identity that the roster, presence, and
invites all agree on.

The mapping lives in players.json next to the bridge config:

	{
	  "100.110.136.90": "bphit4",
	  "100.77.75.68":   "WaddWadd"
	}

Addresses are the ones players connect from — on a VPN those are stable. Anyone
absent from the file still gets a distinct generated name, so an unlisted player
is usable rather than a duplicate.
*/

type playerRoster struct {
	once  sync.Once
	names map[string]string
	path  string
}

var roster playerRoster

// rosterPath is where the name mapping is read from.
func rosterPath() string {
	if custom := os.Getenv("CYPRESS_CFB27_PLAYERS_FILE"); custom != "" {
		return custom
	}
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return "players.json"
	}
	return filepath.Join(appData, "Cypress", "CFB27", "Private", "players.json")
}

// load reads the roster once. A missing file is normal — it just means nobody
// has been named yet.
func (r *playerRoster) load() {
	r.once.Do(func() {
		r.path = rosterPath()
		r.names = map[string]string{}
		raw, err := os.ReadFile(r.path)
		if err != nil {
			return
		}
		var mapping map[string]string
		if err := json.Unmarshal(raw, &mapping); err != nil {
			fmt.Printf("players.json is not valid JSON (%v); falling back to generated names\n", err)
			return
		}
		for address, name := range mapping {
			r.names[strings.ToLower(strings.TrimSpace(address))] = name
		}
		fmt.Printf("player roster: %d named player(s) from %s\n", len(r.names), r.path)
	})
}

// registered holds names players announced for themselves at setup time. The
// Cypress launcher already signs each player in and stores their username on
// their own machine, so the accurate name is the one they report — the file is
// only a fallback for players who never registered.
var (
	registeredMu    sync.RWMutex
	registeredNames = map[string]string{}
)

func addressText(ip uint32) string {
	return net.IPv4(byte(ip>>24), byte(ip>>16), byte(ip>>8), byte(ip)).String()
}

// registerPlayerName records the name a player reported from their own machine,
// and renames the session if one already exists.
//
// The registration request is itself a connection from that player, so a session
// is created for them a fraction of a second BEFORE the name arrives. Recording
// the name without applying it left every player called "LocalPlayer" no matter
// how correctly they registered.
func (s *Service) registerPlayerName(address string, name string) {
	address = strings.ToLower(strings.TrimSpace(address))
	name = strings.TrimSpace(name)
	if address == "" || name == "" {
		return
	}
	registeredMu.Lock()
	registeredNames[address] = name
	registeredMu.Unlock()

	// Persist it. Registrations used to live only in memory, so every server
	// restart wiped them and each player had to re-register or silently revert to
	// a generated name — which happened constantly during development.
	persistPlayerName(address, name)

	renamed := false
	s.playerSessions.Range(func(key, value any) bool {
		ip, ok := key.(uint32)
		if !ok || addressText(ip) != address {
			return true
		}
		if cs, ok := value.(*clientSession); ok {
			cs.identity.personaName = name
			renamed = true
		}
		return false
	})
	if renamed {
		fmt.Printf("player registered: %s is %q (existing session renamed)\n", address, name)
		return
	}
	fmt.Printf("player registered: %s is %q\n", address, name)
}

// nameFor returns a player's name: the one they registered, else the file.
func (r *playerRoster) nameFor(ip uint32) (string, bool) {
	address := addressText(ip)
	registeredMu.RLock()
	name, ok := registeredNames[address]
	registeredMu.RUnlock()
	if ok && name != "" {
		return name, true
	}
	r.load()
	if len(r.names) == 0 {
		return "", false
	}
	name, ok = r.names[address]
	return name, ok
}

// playerNameFor is the display name for a connecting player: the configured one
// when present, otherwise a distinct generated name.
func (s *Service) playerNameFor(ip uint32, slot uint64) string {
	if name, ok := roster.nameFor(ip); ok && name != "" {
		return name
	}
	if slot <= 1 {
		return s.config.Profile
	}
	return fmt.Sprintf("%s-%d", s.config.Profile, slot)
}

// persistPlayerName writes a registration into players.json so it survives a
// server restart. Best-effort: a write failure must never stop a player joining.
func persistPlayerName(address string, name string) {
	roster.load()

	registeredMu.RLock()
	merged := make(map[string]string, len(roster.names)+len(registeredNames))
	for key, value := range roster.names {
		merged[key] = value
	}
	for key, value := range registeredNames {
		merged[key] = value
	}
	registeredMu.RUnlock()

	raw, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return
	}
	path := rosterPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Printf("could not create %s: %v\n", filepath.Dir(path), err)
		return
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		fmt.Printf("could not save the player roster to %s: %v\n", path, err)
		return
	}
	// Keep the in-memory file view current so a later save does not drop this.
	roster.names[address] = name
}
