package mock

import (
	"github.com/antonbier/DreamHub/backend/internal/recfs"
	"github.com/antonbier/DreamHub/backend/internal/webcontrol"
)

// The second mock box (docs/MULTIDEVICE_PLAN.md, M4): a living-room box
// with its own name, two tuners, its own timers, recordings, favourites
// and evening programme, so a box switch in the browser visibly changes
// the content and a view that mixes two boxes up shows in tests. Every
// box added in mock mode is this one (cmd/dreamhub/main.go).

// Channels only the second box has (made-up references, like the radio
// stations above).
var (
	TVProSieben = webcontrol.Service{ServiceReference: "1:0:19:2F44:441:1:C00000:0:0:0:", ServiceName: "ProSieben HD"}
	TVSat1      = webcontrol.Service{ServiceReference: "1:0:19:2F45:441:1:C00000:0:0:0:", ServiceName: "SAT.1 HD"}
	TVVox       = webcontrol.Service{ServiceReference: "1:0:19:2F46:441:1:C00000:0:0:0:", ServiceName: "VOX HD"}
)

// fixture is what differs between the two mock boxes.
type fixture struct {
	device     webcontrol.DeviceInfo
	movies     []webcontrol.Movie
	providers  []provider
	favourites []webcontrol.Service
	schedule   []webcontrol.EPGEvent
	live       string // what plays when nothing was zapped
}

type provider struct {
	name     string
	channels []webcontrol.Service
}

// NewSecond returns the second mock box.
func NewSecond() *Client {
	root := Root
	terra := root + "20260920 2015 - ZDF HD - Terra X_ Das Geheimnis der Pyramiden"
	tt := root + "20260922 2215 - Das Erste HD - Tagesthemen"
	fs := recfs.NewMem(map[string]string{
		terra + ".ts":      "recording",
		terra + ".ts.meta": "1:0:19:2B66:3F3:1:C00000:0:0:0:\nTerra X: Das Geheimnis der Pyramiden\nDokumentation\n1790021700\n",
		terra + ".eit":     "eit",
		tt + ".ts":         "recording",
		tt + ".ts.meta":    "1:0:19:283D:3FB:1:C00000:0:0:0:\nTagesthemen\n\n1790111700\n",
		tt + ".eit":        "eit",
	})
	c := &Client{FS: fs, fx: &fixture{
		device: webcontrol.DeviceInfo{
			EnigmaVersion: "5.0.0r117-gutemine-2024-08-12",
			ImageVersion:  "Experimental 2024-06-01",
			WebifVersion:  "1.9.1",
			DeviceName:    "wohnzimmer",
			Frontends: []webcontrol.Frontend{
				{Name: "Tuner A", Model: "Si2169D (DVB-S2 / DVB-C / DVB-T2)"},
				{Name: "Tuner B", Model: "Si2169D (DVB-S2 / DVB-C / DVB-T2)"},
			},
			HDDs: []webcontrol.HDD{{Model: "WDC WD10SPZX", Capacity: "1 TB", Free: "612.310 GB"}},
		},
		movies: []webcontrol.Movie{
			{ServiceReference: "1:0:0:0:0:0:0:0:0:0:" + terra + ".ts", Title: "Terra X: Das Geheimnis der Pyramiden", Description: "Dokumentation",
				ServiceName: "ZDF HD", Time: 1790021700, Length: "43:20", Filename: "/" + terra + ".ts", FileSize: 1650000000},
			{ServiceReference: "1:0:0:0:0:0:0:0:0:0:" + tt + ".ts", Title: "Tagesthemen",
				ServiceName: "Das Erste HD", Time: 1790111700, Length: "30:00", Filename: "/" + tt + ".ts", FileSize: 980000000},
		},
		providers: []provider{
			{"ARD", []webcontrol.Service{TVDasErste}},
			{"ProSiebenSat.1", []webcontrol.Service{TVProSieben, TVSat1}},
			{"RTL World", []webcontrol.Service{TVVox}},
			{"ZDFvision", []webcontrol.Service{TVZDF}},
		},
		favourites: []webcontrol.Service{TVProSieben, TVSat1, TVZDF, TVDasErste},
		schedule:   secondSchedule(),
		live:       TVZDF.ServiceReference,
	}, timers: []webcontrol.Timer{
		{ServiceReference: TVProSieben.ServiceReference, ServiceName: TVProSieben.ServiceName, Name: "Germany's Next Topmodel", Begin: at(20, 15), End: at(22, 30)},
		{ServiceReference: TVZDF.ServiceReference, ServiceName: TVZDF.ServiceName, Name: "Terra X", Begin: at(19, 30), End: at(20, 15)},
		{ServiceReference: TVSat1.ServiceReference, ServiceName: TVSat1.ServiceName, Name: "Navy CIS", Begin: at(20, 15), End: at(21, 15)},
	}}
	return c
}

func secondSchedule() []webcontrol.EPGEvent {
	ev := func(id int64, svc webcontrol.Service, h, m, minutes int, title, desc string) webcontrol.EPGEvent {
		return webcontrol.EPGEvent{ID: id, Start: at(h, m), Duration: int64(minutes) * 60, Title: title, Description: desc,
			ServiceReference: svc.ServiceReference, ServiceName: svc.ServiceName}
	}
	return []webcontrol.EPGEvent{
		ev(201, TVProSieben, 19, 0, 75, "Galileo", "Wissensmagazin"),
		ev(202, TVProSieben, 20, 15, 135, "Germany's Next Topmodel", "Das Finale"),
		ev(203, TVProSieben, 22, 30, 60, "Late Night Berlin", ""),
		ev(211, TVSat1, 18, 0, 135, "Navy CIS", "Krimiserie"),
		ev(212, TVSat1, 20, 15, 60, "Navy CIS: L.A.", "Krimiserie"),
		ev(213, TVSat1, 21, 15, 105, "The Voice Kids", ""),
		ev(221, TVZDF, 19, 30, 45, "Terra X", "Das Geheimnis der Pyramiden"),
		ev(222, TVZDF, 20, 15, 90, "Der Bergdoktor", "Heimatserie"),
		ev(223, TVZDF, 21, 45, 30, "heute journal", ""),
		ev(231, TVDasErste, 20, 0, 15, "Tagesschau", ""),
		ev(232, TVDasErste, 20, 15, 105, "Die Zeugin", "Fernsehfilm, D 2026"),
		ev(233, TVDasErste, 22, 0, 30, "Tagesthemen", ""),
		ev(241, TVVox, 20, 15, 120, "Das perfekte Dinner", ""),
	}
}
