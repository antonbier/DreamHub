package webcontrol

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
)

// SimilarEvents asks the box for other airings of an EPG event — enigma2's
// own "similar broadcasts" search (the same event repeated; not the next
// episode of a series: a title with "(2/13)" in it found nothing on the
// Solo2). The event itself is not in the answer. ErrNotFound when the box
// has no such call.
//
// On OpenWebif the XML form (/web/epgsimilar) answers "Processing Failed"
// (OWIF 1.5.2 on VTi 15, 2026-10-23: the handler returns a list where its
// template expects a dict), so the JSON form /api/epgsimilar is used there;
// a Dreambox's WebControl has XML only. A Dreambox One (DreamOS, 2026-10-23)
// answers a GET with the parameters, and its list is wider than a VTi's: past
// airings, unsorted, and the same programme on regional feeds of the channel
// ("SAT.1 Bayern", "RAS Das Erste HD", "ARD-Test-1") — most of them not in
// any bouquet of the user. The caller sorts and drops what is over.
func (c *HTTPClient) SimilarEvents(ctx context.Context, serviceReference string, eventID int64) ([]EPGEvent, error) {
	var events []EPGEvent
	if c.runsOpenWebif(ctx) {
		q := url.Values{"sRef": {serviceReference}, "eventid": {strconv.FormatInt(eventID, 10)}}
		body, err := c.get(ctx, "/api/epgsimilar?"+q.Encode())
		if err != nil {
			return nil, err
		}
		defer body.Close()
		if events, err = decodeSimilarJSON(body); err != nil {
			return nil, err
		}
	} else {
		q := url.Values{"sRef": {serviceReference}, "eventid": {strconv.FormatInt(eventID, 10)}}
		var list epgEventList
		if err := c.getXML(ctx, "/web/epgsimilar?"+q.Encode(), &list); err != nil {
			return nil, err
		}
		events = list.Events
	}
	out := events[:0]
	for _, e := range events {
		if !strings.HasPrefix(e.ServiceReference, "-1:") && !(e.ID == eventID && e.ServiceReference == serviceReference) {
			out = append(out, e)
		}
	}
	return out, nil
}

// decodeSimilarJSON reads OpenWebif's /api/epgsimilar: a list of events
// (fixture vti15_epgsimilar.json), or an object with "result": false and a
// message when the parameters are wrong — no events then.
func decodeSimilarJSON(r io.Reader) ([]EPGEvent, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("webcontrol: reading /api/epgsimilar response: %w", err)
	}
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) {
		return nil, nil
	}
	var raw []struct {
		ID          int64  `json:"id"`
		Begin       int64  `json:"begin_timestamp"`
		Duration    int64  `json:"duration_sec"`
		Title       string `json:"title"`
		Description string `json:"shortdesc"`
		Ref         string `json:"sref"`
		Name        string `json:"sname"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("webcontrol: parsing /api/epgsimilar response: %w", err)
	}
	out := make([]EPGEvent, 0, len(raw))
	for _, e := range raw {
		out = append(out, EPGEvent{
			ID: e.ID, Start: e.Begin, Duration: e.Duration, Title: e.Title,
			Description: e.Description, ServiceReference: e.Ref, ServiceName: e.Name,
		})
	}
	return out, nil
}
