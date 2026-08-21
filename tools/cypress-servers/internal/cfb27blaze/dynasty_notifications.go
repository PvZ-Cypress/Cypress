package cfb27blaze

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"path"
	"sort"

	"cypress-servers/internal/blaze"
)

//go:embed fixtures/dynasty-*-notifications-*.bin
var dynastyNotificationFixtures embed.FS

func capturedDynastyNotificationBatches() map[route]map[uint32][]blaze.Frame {
	names, err := dynastyNotificationFixtures.ReadDir("fixtures")
	if err != nil {
		panic(fmt.Sprintf("read embedded Dynasty notifications: %v", err))
	}
	sort.Slice(names, func(i, j int) bool { return names[i].Name() < names[j].Name() })
	batches := make(map[route]map[uint32][]blaze.Frame)
	for _, entry := range names {
		var command uint16
		var occurrence uint32
		if _, err := fmt.Sscanf(entry.Name(), "dynasty-%d-notifications-%03d.bin", &command, &occurrence); err != nil {
			continue
		}
		r := route{ComponentBootStatus, command}
		if batches[r] == nil {
			batches[r] = make(map[uint32][]blaze.Frame)
		}
		data, err := dynastyNotificationFixtures.ReadFile(path.Join("fixtures", entry.Name()))
		if err != nil {
			panic(fmt.Sprintf("read embedded Dynasty notification batch %d: %v", occurrence, err))
		}
		reader := bytes.NewReader(data)
		for reader.Len() > 0 {
			frame, err := blaze.ReadFrame(reader)
			if err != nil {
				if err == io.EOF {
					break
				}
				panic(fmt.Sprintf("decode embedded Dynasty notification batch %d: %v", occurrence, err))
			}
			batches[r][occurrence] = append(batches[r][occurrence], frame)
		}
	}
	return batches
}

func localizeDynastyNotificationPayload(payload []byte, leagueID int64) ([]byte, error) {
	if len(payload) < 5 || !bytes.Equal(payload[:3], []byte{0xb2, 0x7a, 0x64}) || payload[3] != byte(blaze.TypeInteger) {
		return nil, fmt.Errorf("Dynasty notification does not start with integer LGID")
	}
	end := 4
	for {
		if end >= len(payload) {
			return nil, fmt.Errorf("Dynasty notification has truncated LGID")
		}
		value := payload[end]
		end++
		if value&0x80 == 0 {
			break
		}
	}
	if leagueID < 0 {
		return nil, fmt.Errorf("Dynasty league ID must be non-negative")
	}
	encoded := make([]byte, 0, 10)
	magnitude := uint64(leagueID)
	first := byte(magnitude & 0x3f)
	magnitude >>= 6
	if magnitude != 0 {
		first |= 0x80
	}
	encoded = append(encoded, first)
	for magnitude != 0 {
		next := byte(magnitude & 0x7f)
		magnitude >>= 7
		if magnitude != 0 {
			next |= 0x80
		}
		encoded = append(encoded, next)
	}
	localized := make([]byte, 0, len(payload)-(end-4)+len(encoded))
	localized = append(localized, payload[:4]...)
	localized = append(localized, encoded...)
	localized = append(localized, payload[end:]...)
	return localized, nil
}
