package mock

import (
	"context"
	"fmt"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// The mock box's BouquetEditor plugin (webcontrol.BouquetEditor): bouquets
// become editable copies on first change. Its behaviour is the one
// ChannelHub assumes — positions count from 0 with headings, a new entry
// goes before beforeRef or at the end — which the box still has to
// confirm (docs/CHANNELHUB_PLAN.md).

// editorState is everything a backup holds.
type editorState struct {
	tv, radio []webcontrol.Service
	entries   map[string][]webcontrol.Service
}

// Bouquets lists the TV bouquets.
func (c *Client) Bouquets(ctx context.Context) ([]webcontrol.Service, error) {
	if err := c.disturb(ctx); err != nil {
		return nil, err
	}
	c.mu.Lock()
	if c.tvBouquets != nil {
		out := append([]webcontrol.Service(nil), c.tvBouquets...)
		c.mu.Unlock()
		return out, nil
	}
	c.mu.Unlock()
	return c.baseBouquets(ctx)
}

// Services lists a bouquet's entries (or any other list).
func (c *Client) Services(ctx context.Context, bouquetRef string) ([]webcontrol.Service, error) {
	if err := c.disturb(ctx); err != nil {
		return nil, err
	}
	c.mu.Lock()
	if list, ok := c.entries[bouquetRef]; ok {
		out := append([]webcontrol.Service(nil), list...)
		c.mu.Unlock()
		return out, nil
	}
	if bouquetRef == webcontrol.RadioRoot && c.radioBouquets != nil {
		out := append([]webcontrol.Service(nil), c.radioBouquets...)
		c.mu.Unlock()
		return out, nil
	}
	c.mu.Unlock()
	return c.baseServices(ctx, bouquetRef)
}

// editable makes sure the bouquet lists and the bouquet's entries are
// copies the mock can change. Called without the lock held.
func (c *Client) editable(ctx context.Context, bouquetRef string) error {
	if err := c.disturb(ctx); err != nil {
		return err
	}
	tv, _ := c.Bouquets(ctx)
	radio, _ := c.Services(ctx, webcontrol.RadioRoot)
	var list []webcontrol.Service
	if bouquetRef != "" {
		var err error
		if list, err = c.Services(ctx, bouquetRef); err != nil {
			return err
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tvBouquets == nil {
		c.tvBouquets = tv
	}
	if c.radioBouquets == nil {
		c.radioBouquets = radio
	}
	if c.entries == nil {
		c.entries = map[string][]webcontrol.Service{}
	}
	if _, ok := c.entries[bouquetRef]; !ok && bouquetRef != "" {
		c.entries[bouquetRef] = list
	}
	return nil
}

func (c *Client) failing(op string) error {
	if c.FailEditor == op {
		return &webcontrol.RejectedError{Op: op, Text: "mock: refused"}
	}
	return nil
}

func indexOf(list []webcontrol.Service, ref string) int {
	for i, s := range list {
		if s.ServiceReference == ref {
			return i
		}
	}
	return -1
}

func (c *Client) EditorBackup(ctx context.Context, filename string) error {
	if err := c.disturb(ctx); err != nil {
		return err
	}
	if err := c.failing("backup"); err != nil {
		return err
	}
	if err := c.editable(ctx, ""); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	st := editorState{tv: append([]webcontrol.Service(nil), c.tvBouquets...), radio: append([]webcontrol.Service(nil), c.radioBouquets...),
		entries: map[string][]webcontrol.Service{}}
	for k, v := range c.entries {
		st.entries[k] = append([]webcontrol.Service(nil), v...)
	}
	if c.backups == nil {
		c.backups = map[string]editorState{}
	}
	c.backups[filename] = st
	return nil
}

func (c *Client) EditorRestore(ctx context.Context, filename string) error {
	if err := c.disturb(ctx); err != nil {
		return err
	}
	if err := c.failing("restore"); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	st, ok := c.backups[filename]
	if !ok {
		return &webcontrol.RejectedError{Op: "restore", Text: "no such backup"}
	}
	c.tvBouquets, c.radioBouquets = st.tv, st.radio
	c.entries = map[string][]webcontrol.Service{}
	for k, v := range st.entries {
		c.entries[k] = append([]webcontrol.Service(nil), v...)
	}
	c.Restores++
	return nil
}

func bouquetFile(name string, radio bool) string {
	ext := "tv"
	if radio {
		ext = "radio"
	}
	return fmt.Sprintf(`1:7:%d:0:0:0:0:0:0:0:FROM BOUQUET "userbouquet.%s.%s" ORDER BY bouquet`,
		map[bool]int{false: 1, true: 2}[radio], strings.ToLower(strings.ReplaceAll(name, " ", "")), ext)
}

func (c *Client) AddBouquet(ctx context.Context, name string, radio bool) error {
	if err := c.disturb(ctx); err != nil {
		return err
	}
	if err := c.failing("addbouquet"); err != nil {
		return err
	}
	if err := c.editable(ctx, ""); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	b := webcontrol.Service{ServiceReference: bouquetFile(name, radio), ServiceName: name}
	if radio {
		c.radioBouquets = append(c.radioBouquets, b)
	} else {
		c.tvBouquets = append(c.tvBouquets, b)
	}
	c.entries[b.ServiceReference] = []webcontrol.Service{}
	return nil
}

func (c *Client) RemoveBouquet(ctx context.Context, bouquetRef string, radio bool) error {
	if err := c.disturb(ctx); err != nil {
		return err
	}
	if err := c.failing("removebouquet"); err != nil {
		return err
	}
	if err := c.editable(ctx, ""); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	list := &c.tvBouquets
	if radio {
		list = &c.radioBouquets
	}
	i := indexOf(*list, bouquetRef)
	if i < 0 {
		return &webcontrol.RejectedError{Op: "removebouquet", Text: "no such bouquet"}
	}
	*list = append((*list)[:i], (*list)[i+1:]...)
	delete(c.entries, bouquetRef)
	return nil
}

func (c *Client) RenameBouquet(ctx context.Context, bouquetRef, newName string, radio bool) error {
	if err := c.disturb(ctx); err != nil {
		return err
	}
	if err := c.failing("renamebouquet"); err != nil {
		return err
	}
	if err := c.editable(ctx, ""); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	list := c.tvBouquets
	if radio {
		list = c.radioBouquets
	}
	i := indexOf(list, bouquetRef)
	if i < 0 {
		return &webcontrol.RejectedError{Op: "renameservice", Text: "no such bouquet"}
	}
	list[i].ServiceName = newName
	return nil
}

func (c *Client) insert(bouquetRef string, s webcontrol.Service, beforeRef string) {
	list := c.entries[bouquetRef]
	at := len(list)
	if beforeRef != "" {
		if i := indexOf(list, beforeRef); i >= 0 {
			at = i
		}
	}
	list = append(list, webcontrol.Service{})
	copy(list[at+1:], list[at:])
	list[at] = s
	c.entries[bouquetRef] = list
}

func (c *Client) AddToBouquet(ctx context.Context, bouquetRef, ref, beforeRef, name string) error {
	if err := c.disturb(ctx); err != nil {
		return err
	}
	if err := c.failing("addservicetobouquet"); err != nil {
		return err
	}
	if err := c.editable(ctx, bouquetRef); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.insert(bouquetRef, webcontrol.Service{ServiceReference: ref, ServiceName: name}, beforeRef)
	return nil
}

func (c *Client) RemoveFromBouquet(ctx context.Context, bouquetRef, ref string) error {
	if err := c.disturb(ctx); err != nil {
		return err
	}
	if err := c.failing("removeservice"); err != nil {
		return err
	}
	if err := c.editable(ctx, bouquetRef); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	list := c.entries[bouquetRef]
	i := indexOf(list, ref)
	if i < 0 {
		return &webcontrol.RejectedError{Op: "removeservice", Text: "not in the bouquet"}
	}
	c.entries[bouquetRef] = append(list[:i], list[i+1:]...)
	return nil
}

func (c *Client) MoveInBouquet(ctx context.Context, bouquetRef, ref string, position int, radio bool) error {
	if err := c.disturb(ctx); err != nil {
		return err
	}
	if err := c.failing("moveservice"); err != nil {
		return err
	}
	if err := c.editable(ctx, bouquetRef); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	list := c.entries[bouquetRef]
	i := indexOf(list, ref)
	if i < 0 || position < 0 || position >= len(list) {
		return &webcontrol.RejectedError{Op: "moveservice", Text: "bad position"}
	}
	s := list[i]
	list = append(list[:i], list[i+1:]...)
	list = append(list, webcontrol.Service{})
	copy(list[position+1:], list[position:])
	list[position] = s
	c.entries[bouquetRef] = list
	return nil
}

func (c *Client) RenameInBouquet(ctx context.Context, bouquetRef, ref, beforeRef, newName string, radio bool) error {
	if err := c.disturb(ctx); err != nil {
		return err
	}
	if err := c.failing("renameservice"); err != nil {
		return err
	}
	if err := c.editable(ctx, bouquetRef); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	list := c.entries[bouquetRef]
	i := indexOf(list, ref)
	if i < 0 {
		return &webcontrol.RejectedError{Op: "renameservice", Text: "not in the bouquet"}
	}
	list[i].ServiceName = newName
	return nil
}

func (c *Client) AddMarker(ctx context.Context, bouquetRef, name, beforeRef string) error {
	if err := c.disturb(ctx); err != nil {
		return err
	}
	if err := c.failing("addmarkertobouquet"); err != nil {
		return err
	}
	if err := c.editable(ctx, bouquetRef); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.markers++
	c.insert(bouquetRef, webcontrol.Service{ServiceReference: fmt.Sprintf("1:64:%X:0:0:0:0:0:0:0::%s", c.markers, name), ServiceName: name}, beforeRef)
	return nil
}

var _ webcontrol.BouquetEditor = (*Client)(nil)
