// Command dreamhub-ext-ewelink is DreamHub's reference extension
// (docs/EXTENSIONHUB_PLAN.md, E4): eWeLink/Sonoff devices with "LAN
// control" switched on, read and switched on the local network only —
// no eWeLink cloud at run time. It finds devices over mDNS, decrypts
// their announced state with each device's key, and registers with
// DreamHub as one card of status, readings and switch actions.
//
// It talks to DreamHub only through the public extension contract
// (docs/BUILDING_EXTENSIONS.md) and imports nothing from DreamHub's own
// code — it doubles as the worked example for other extensions.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const schemaVersion = 3

func main() {
	path := os.Getenv("DREAMHUB_EWELINK_CONFIG")
	if path == "" {
		path = "/var/lib/dreamhub-ext-ewelink/config.json"
	}
	cfg, err := loadConfig(path)
	if err != nil {
		log.Fatalf("config %s: %v", path, err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store := NewConfigStore(path, cfg)
	devices := NewDevices(store)
	cloud, energy := NewCloud(), NewEnergyStore()
	go refreshLoop(ctx, devices)
	go energyLoop(ctx, store, cloud, energy)
	go registerLoop(ctx, cfg)

	srv := &http.Server{Addr: cfg.Listen, Handler: (&Server{cfg: store, devices: devices, cloud: cloud, energy: energy}).Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shut, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shut)
	}()
	log.Printf("dreamhub-ext-ewelink listening on %s, %d device(s) configured", cfg.Listen, len(cfg.Devices))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func refreshLoop(ctx context.Context, d *Devices) {
	for {
		if err := d.Refresh(ctx); err != nil {
			log.Printf("browse: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
		}
	}
}

// registerLoop registers now and again every minute: DreamHub keeps
// registrations in memory only, so after its own restart it needs this.
func registerLoop(ctx context.Context, cfg Config) {
	body, _ := json.Marshal(map[string]any{
		"id": "ewelink", "name": "eWeLink", "icon": "control", "admin": false,
		"schemaVersion": schemaVersion, "settings": true, "baseUrl": "http://" + cfg.Listen,
	})
	client := &http.Client{Timeout: 5 * time.Second}
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, cfg.DreamHub+"/api/v1/extensions/register", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-DreamHub-Request", "1") // DreamHub's CSRF marker on every POST
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("register: %v", err)
		} else {
			if resp.StatusCode != http.StatusOK {
				log.Printf("register: DreamHub answered %d", resp.StatusCode)
			}
			resp.Body.Close()
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Minute):
		}
	}
}
