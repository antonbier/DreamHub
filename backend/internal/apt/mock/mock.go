// Package mock implements apt.Lister in memory: a handful of packages
// across the categories actually seen on the reference box
// (docs/ADMINHUB_PLAN.md: "Section field confirmed live, 2026-10-11"), one
// of them upgradable, so the browser has something to show.
package mock

import (
	"context"

	"github.com/antonbier/DreamHub/backend/internal/apt"
)

// Lister is an in-memory apt.Lister.
type Lister struct {
	// Packages overrides the defaults below (tests).
	Packages []apt.Package
}

var _ apt.Lister = (*Lister)(nil)

func New() *Lister {
	return &Lister{Packages: []apt.Package{
		{Name: "dreamhub", Version: "0.3.0", Category: "web", Description: "DreamHub web interface"},
		{Name: "enigma2", Version: "4.6-r1", Category: "base", Description: "Enigma2 set-top box middleware"},
		{Name: "enigma2-plugin-skincomponents-dreamhubbutton", Version: "1.0", Category: "plugin-skincomponents", Description: "Skin button for DreamHub"},
		{Name: "gp4-picon-hotbird13e", Version: "20260901", Category: "base", Description: "Picons for Hotbird 13E"},
		{Name: "dropbear", Version: "2022.83", Category: "console/network", Description: "Lightweight SSH server"},
		{Name: "python3-core", Version: "3.11.6-r2", Category: "devel/python", Description: "Minimal Python 3 interpreter"},
		{Name: "oscam-cycecplus", Version: "11742", Category: "console/network", Description: "Open Source Conditional Access Module", Upgradable: true, AvailableVersion: "11760"},
	}}
}

func (l *Lister) List(ctx context.Context) ([]apt.Package, error) {
	return l.Packages, nil
}
