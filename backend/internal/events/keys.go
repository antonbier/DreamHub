package events

// Event keys: what the bus publishes, the notification inbox stores and
// push forwards. Each belongs to a hub's area and is declared in its
// descriptor (hub.Descriptor.Emits); the web UI's texts are under
// "notification.<key>" in the frontend locales, push's in
// internal/push/messages.go — a test checks that every key has both.
const (
	RecordingStarted  = "recording.started"    // the poller, for TimerHub
	RecordingFinished = "recording.finished"   // the poller, for TimerHub
	TimerConflict     = "timer.conflict"       // the poller, for TimerHub
	DiskAboveLimit    = "disk.aboveThreshold"  // the poller, for MediaHub
	MissingAiring     = "series.missingAiring" // SeriesHub's watch
	ServiceFailed     = "service.failed"       // AdminHub's watch
	ServiceRecovered  = "service.recovered"    // AdminHub's watch
)

// All lists every key above.
var All = []string{RecordingStarted, RecordingFinished, TimerConflict, DiskAboveLimit, MissingAiring, ServiceFailed, ServiceRecovered}
