package newborn

import (
	"fmt"
	"strings"
	"time"

	"ai-arena/internal/worldstate"
)

type WorldBridge struct {
	store *worldstate.Store
}

func NewWorldBridge(root string) *WorldBridge {
	return &WorldBridge{store: worldstate.New(root)}
}

func (w *WorldBridge) RecordResidentMessage(profile ResidentProfile, body string, now time.Time) (string, error) {
	msg, err := w.store.AppendResidentToChenglin(profile.Name, body, now)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("message delivered to Chenglin and recorded in world state:\nmessage_id=%s\ncreated_at=%s\nbody=%s", msg.ID, msg.CreatedAt, msg.Body), nil
}

func (w *WorldBridge) BuildResidentWorldContext(profile ResidentProfile, limit int) string {
	messages, err := w.store.ReadRecentForResident(profile.Name, limit)
	if err != nil || len(messages) == 0 {
		return "Recent world messages involving you: none recorded."
	}

	lines := []string{"Recent world messages involving you:"}
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		lines = append(lines, fmt.Sprintf("- [%s] %s -> %s: %s", msg.CreatedAt, msg.From, msg.To, oneLine(msg.Body)))
	}
	return strings.Join(lines, "\n")
}

func oneLine(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 280 {
		return s[:280] + "..."
	}
	return s
}
