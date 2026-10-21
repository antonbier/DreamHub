package webcontrol

import (
	"context"
	"strconv"
	"strings"
)

// CurrentService is what the box shows right now: the service (a channel,
// a radio station or a recording being played back), its programme now
// and next where there's EPG, and the volume.
type CurrentService struct {
	ServiceReference string    `json:"serviceReference"`
	ServiceName      string    `json:"serviceName"`
	ProviderName     string    `json:"providerName"`
	Now              *EPGEvent `json:"now"`
	Next             *EPGEvent `json:"next"`
	Volume           int       `json:"volume"`
	Muted            bool      `json:"muted"`
}

// Current reads /web/getcurrent (fixture getcurrent.xml, captured live):
// the service, then an event list — now first, then next — and the
// volume. Event fields are read as text and converted here, the same way
// as EPGNowNext, since a service without EPG answers them as "None".
func (c *HTTPClient) Current(ctx context.Context) (*CurrentService, error) {
	var res struct {
		Service struct {
			Reference string `xml:"e2servicereference"`
			Name      string `xml:"e2servicename"`
			Provider  string `xml:"e2providername"`
		} `xml:"e2service"`
		Events []struct {
			ID          string `xml:"e2eventid"`
			Start       string `xml:"e2eventstart"`
			Duration    string `xml:"e2eventduration"`
			Title       string `xml:"e2eventtitle"`
			Description string `xml:"e2eventdescription"`
		} `xml:"e2eventlist>e2event"`
		Volume struct {
			Current string `xml:"e2current"`
			Muted   string `xml:"e2ismuted"`
		} `xml:"e2volume"`
	}
	if err := c.getXML(ctx, "/web/getcurrent", &res); err != nil {
		return nil, err
	}
	out := &CurrentService{
		ServiceReference: strings.TrimSpace(res.Service.Reference),
		ServiceName:      strings.TrimSpace(res.Service.Name),
		ProviderName:     strings.TrimSpace(res.Service.Provider),
		Muted:            strings.EqualFold(strings.TrimSpace(res.Volume.Muted), "True"),
	}
	out.Volume, _ = strconv.Atoi(strings.TrimSpace(res.Volume.Current))
	if strings.TrimSpace(res.Volume.Current) == "" {
		// OpenWebif's getcurrent carries no <e2volume> (VTi 15, 2026-10-21);
		// /web/vol has it. Best effort: the volume is not worth failing for.
		var vol struct {
			Current string `xml:"e2current"`
			Muted   string `xml:"e2ismuted"`
		}
		if c.getXML(ctx, "/web/vol", &vol) == nil {
			out.Volume, _ = strconv.Atoi(strings.TrimSpace(vol.Current))
			out.Muted = strings.EqualFold(strings.TrimSpace(vol.Muted), "True")
		}
	}
	for _, e := range res.Events {
		id, err1 := strconv.ParseInt(strings.TrimSpace(e.ID), 10, 64)
		start, err2 := strconv.ParseInt(strings.TrimSpace(e.Start), 10, 64)
		dur, err3 := strconv.ParseInt(strings.TrimSpace(e.Duration), 10, 64)
		if err1 != nil || err2 != nil || err3 != nil {
			continue // "None": no EPG
		}
		ev := &EPGEvent{ID: id, Start: start, Duration: dur, Title: strings.TrimSpace(e.Title), Description: strings.TrimSpace(e.Description),
			ServiceReference: out.ServiceReference, ServiceName: out.ServiceName}
		if out.Now == nil {
			out.Now = ev
		} else if out.Next == nil {
			out.Next = ev
		}
	}
	return out, nil
}

// IsRecordingReference reports whether ref plays back a file (a recording,
// or any media file) rather than a live service: its path follows the ten
// numeric fields ("1:0:0:0:0:0:0:0:0:0:/media/hdd/movie/….ts").
func IsRecordingReference(ref string) bool {
	parts := strings.SplitN(ref, ":", 11)
	return len(parts) == 11 && strings.HasPrefix(parts[10], "/")
}

// Raw returns a WebControl answer as the box sent it — for the self-test
// (internal/hubs/selftest.go), which keeps them as test fixtures. Read-only
// GETs only; nothing here parses or interprets the body.
func (c *HTTPClient) Raw(ctx context.Context, path string) ([]byte, error) {
	return c.coalescedGet(ctx, path)
}
