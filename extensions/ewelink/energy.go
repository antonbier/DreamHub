package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"strconv"
	"sync"
	"time"
)

// Energy is what the cloud last said about one plug's consumption and
// live values. The LAN announcement carries the live switch state; the
// history (today, this month) exists only in the cloud.
type Energy struct {
	At        time.Time
	PowerW    *float64
	VoltageV  *float64
	CurrentA  *float64
	TodayKwh  *float64
	MonthKwh  *float64
	Days      []float64 // today first
	HasKwh    bool
	LastError string
}

// EnergyStore keeps the latest Energy per device.
type EnergyStore struct {
	mu sync.Mutex
	m  map[string]Energy
}

func NewEnergyStore() *EnergyStore { return &EnergyStore{m: map[string]Energy{}} }

func (e *EnergyStore) Get(id string) (Energy, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	v, ok := e.m[id]
	return v, ok
}

func (e *EnergyStore) set(id string, v Energy) {
	e.mu.Lock()
	e.m[id] = v
	e.mu.Unlock()
}

// number reads a state value that the cloud sends as a string or a number.
func number(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case string:
		f, err := strconv.ParseFloat(x, 64)
		return f, err == nil
	}
	return 0, false
}

func ptr(f float64) *float64 { return &f }

// monthTotal adds this month's days (today is days[0], so the month so
// far is the first day-of-month entries).
func monthTotal(days []float64, now time.Time) float64 {
	n := now.Day()
	if n > len(days) {
		n = len(days)
	}
	var sum float64
	for _, d := range days[:n] {
		sum += d
	}
	return math.Round(sum*100) / 100
}

// kwhWait is how long the device gets to report its history after being asked.
var kwhWait = 3 * time.Second

// readEnergy asks the cloud about one device. The consumption history
// needs a request first and shows up in the state a moment later.
func readEnergy(ctx context.Context, cl *Cloud, cfg CloudConfig, id string, now time.Time) Energy {
	out := Energy{At: now}
	if err := cl.AskKwh(ctx, cfg, id); err != nil {
		out.LastError = err.Error()
	} else {
		select {
		case <-ctx.Done():
		case <-time.After(kwhWait):
		}
	}
	params, err := cl.Params(ctx, cfg, id)
	if err != nil {
		out.LastError = err.Error()
		return out
	}
	if v, ok := number(params["power"]); ok {
		out.PowerW = ptr(v)
	}
	if v, ok := number(params["voltage"]); ok {
		out.VoltageV = ptr(v)
	}
	if v, ok := number(params["current"]); ok {
		out.CurrentA = ptr(v)
	}
	if s, ok := params["hundredDaysKwhData"].(string); ok {
		if days := ParseKwh(s); len(days) > 0 {
			out.Days, out.HasKwh = days, true
			out.TodayKwh = ptr(days[0])
			out.MonthKwh = ptr(monthTotal(days, now))
		}
	}
	return out
}

// energyLoop reads every configured device once a quarter of an hour
// while a login is stored (the free application has 50 000 calls a month;
// this is about 6 a device an hour).
func energyLoop(ctx context.Context, store *ConfigStore, cl *Cloud, energy *EnergyStore) {
	for {
		energyOnce(ctx, store, cl, energy)
		select {
		case <-ctx.Done():
			return
		case <-time.After(15 * time.Minute):
		}
	}
}

func energyOnce(ctx context.Context, store *ConfigStore, cl *Cloud, energy *EnergyStore) {
	cfg := store.Get()
	if !cfg.Cloud.LoggedIn() || len(cfg.Devices) == 0 {
		return
	}
	fresh, changed, err := cl.Fresh(ctx, cfg.Cloud)
	if err != nil {
		log.Printf("eWeLink cloud: %v", err)
		for _, d := range cfg.Devices {
			energy.set(d.ID, Energy{At: cl.Now(), LastError: err.Error()})
		}
		return
	}
	if changed {
		if err := store.Update(func(c *Config) error { c.Cloud = fresh; return nil }); err != nil {
			log.Printf("eWeLink cloud: storing the new token: %v", err)
		}
	}
	for _, d := range cfg.Devices {
		e := readEnergy(ctx, cl, fresh, d.ID, cl.Now())
		if e.LastError != "" {
			log.Printf("eWeLink cloud: %s: %s", d.ID, e.LastError)
		}
		energy.set(d.ID, e)
	}
}

// kwh formats a consumption with two decimals.
func kwh(f float64) string { return fmt.Sprintf("%.2f kWh", f) }
