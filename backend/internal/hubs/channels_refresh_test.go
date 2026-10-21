package hubs

import (
	"context"
	"errors"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/store"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol/mock"
)

// strugglingBox answers the lists but not the per-provider reads, like a
// box that went away under the refresh.
type strugglingBox struct {
	*mock.Client
	providerReads int
}

func (b *strugglingBox) Services(ctx context.Context, ref string) ([]webcontrol.Service, error) {
	switch ref {
	case webcontrol.TVAll, webcontrol.RadioAll, webcontrol.TVProviders, webcontrol.RadioProviders:
		return b.Client.Services(ctx, ref)
	}
	b.providerReads++
	return nil, errors.New("box does not answer")
}

// The snapshot still records the names, and the box is asked at most three
// times in a row for a provider's list before the refresh gives up on them.
func TestChannels_RefreshStopsAskingAStrugglingBox(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	wc := &strugglingBox{Client: mock.New()}
	h := &Channels{WC: wc, Store: st}
	if err := h.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	// TV providers, then the one radio provider: 3 tries each at most.
	if wc.providerReads > 6 {
		t.Errorf("%d provider reads, want at most 6", wc.providerReads)
	}
	seen, _, err := st.SeenChannels(context.Background())
	if err != nil || len(seen) == 0 {
		t.Errorf("snapshot: %d channels, err %v", len(seen), err)
	}
}
