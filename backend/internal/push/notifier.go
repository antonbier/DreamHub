package push

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/antonbier/DreamHub/backend/internal/connector"
	"github.com/antonbier/DreamHub/backend/internal/events"
	"github.com/antonbier/DreamHub/backend/internal/settings"
	"github.com/antonbier/DreamHub/backend/internal/store"
)

// Notifier forwards events from the core bus to the phone: every event
// whose kind is switched on in Settings → Push and that has a text
// (Compose), as long as a topic is set. Sending is best effort — a push
// that doesn't get through is logged, the event itself stays in the bell.
type Notifier struct {
	Bus interface {
		Subscribe() (<-chan store.Notification, func())
	}
	Settings *settings.Service
	// User is whose language the texts are in (v1 has one user).
	User   store.User
	Client Client
	Logger *slog.Logger
}

// eventSwitch is the setting that turns each event kind's push on.
var eventSwitch = map[string]string{
	events.RecordingStarted:  connector.KeyPushRecordingStarted,
	events.RecordingFinished: connector.KeyPushRecordingFinished,
	events.DiskAboveLimit:    connector.KeyPushDisk,
	events.TimerConflict:     connector.KeyPushConflict,
	events.MissingAiring:     connector.KeyPushMissingEpisode,
	events.ServiceFailed:     connector.KeyPushService,
	events.ServiceRecovered:  connector.KeyPushService,
}

// Run forwards until ctx is done; meant for its own goroutine.
func (n *Notifier) Run(ctx context.Context) {
	events, cancel := n.Bus.Subscribe()
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-events:
			sw, ok := eventSwitch[e.Key]
			if !ok {
				continue
			}
			v, err := n.Settings.Values(ctx, n.User)
			if err != nil || !v.Bool(sw) || v.String(connector.KeyPushTopic) == "" {
				continue
			}
			if err := n.send(ctx, v, e.Key, e.Params); err != nil && n.Logger != nil {
				n.Logger.Warn("push not sent", "event", e.Key, "error", err)
			}
		}
	}
}

// Test sends a test message with the current settings.
func (n *Notifier) Test(ctx context.Context) error {
	v, err := n.Settings.Values(ctx, n.User)
	if err != nil {
		return err
	}
	return n.send(ctx, v, "push.test", json.RawMessage(`{}`))
}

func (n *Notifier) send(ctx context.Context, v settings.Values, key string, params json.RawMessage) error {
	m, ok := Compose(key, params, v.String(settings.KeyLanguage), time.Local)
	if !ok {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return n.Client.Send(ctx, Config{Server: v.String(connector.KeyPushServer), Topic: v.String(connector.KeyPushTopic), Token: v.String(connector.KeyPushToken)}, m)
}
