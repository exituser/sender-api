package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var httpRequests atomic.Uint64
var httpServerErrors atomic.Uint64

var customRegistry struct {
	sync.RWMutex
	counters map[string]*atomic.Uint64
	gauges   map[string]*atomic.Int64
}

func AddCounter(name string, delta uint64) {
	name = metricName(name)
	if name == "" || delta == 0 {
		return
	}
	customRegistry.Lock()
	if customRegistry.counters == nil {
		customRegistry.counters = make(map[string]*atomic.Uint64)
	}
	counter := customRegistry.counters[name]
	if counter == nil {
		counter = &atomic.Uint64{}
		customRegistry.counters[name] = counter
	}
	customRegistry.Unlock()
	counter.Add(delta)
}

func SetGauge(name string, value int64) {
	name = metricName(name)
	if name == "" {
		return
	}
	customRegistry.Lock()
	if customRegistry.gauges == nil {
		customRegistry.gauges = make(map[string]*atomic.Int64)
	}
	gauge := customRegistry.gauges[name]
	if gauge == nil {
		gauge = &atomic.Int64{}
		customRegistry.gauges[name] = gauge
	}
	customRegistry.Unlock()
	gauge.Store(value)
}

func metricName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	for index, char := range name {
		valid := (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || char == '_' || (index > 0 && char >= '0' && char <= '9')
		if !valid {
			return ""
		}
	}
	return name
}

type WorkerMetrics struct {
	name                               string
	started, completed, failed         atomic.Uint64
	inFlight                           atomic.Int64
	visibilityExtend, visibilityErrors atomic.Uint64
	oldestAgeNanos                     atomic.Int64
	lastHeartbeat                      atomic.Int64
}

var workerRegistry struct {
	sync.RWMutex
	items map[string]*WorkerMetrics
}

func NewWorkerMetrics(name string) *WorkerMetrics {
	name = strings.TrimSpace(name)
	for _, char := range name {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '_' {
			name = "worker"
			break
		}
	}
	if name == "" {
		name = "worker"
	}
	m := &WorkerMetrics{name: name}
	m.Heartbeat()
	workerRegistry.Lock()
	if workerRegistry.items == nil {
		workerRegistry.items = make(map[string]*WorkerMetrics)
	}
	workerRegistry.items[name] = m
	workerRegistry.Unlock()
	return m
}

func (m *WorkerMetrics) Start()                     { m.started.Add(1); m.inFlight.Add(1); m.Heartbeat() }
func (m *WorkerMetrics) Complete()                  { m.completed.Add(1); m.inFlight.Add(-1); m.Heartbeat() }
func (m *WorkerMetrics) Fail()                      { m.failed.Add(1); m.inFlight.Add(-1); m.Heartbeat() }
func (m *WorkerMetrics) Heartbeat()                 { m.lastHeartbeat.Store(time.Now().UnixNano()) }
func (m *WorkerMetrics) VisibilityExtended()        { m.visibilityExtend.Add(1) }
func (m *WorkerMetrics) VisibilityExtensionFailed() { m.visibilityErrors.Add(1) }
func (m *WorkerMetrics) ObserveAge(age time.Duration) {
	for {
		old := m.oldestAgeNanos.Load()
		if int64(age) <= old || m.oldestAgeNanos.CompareAndSwap(old, int64(age)) {
			return
		}
	}
}

type WorkerSnapshot struct {
	Name                                                                                  string
	Started, Completed, Failed, InFlight, VisibilityExtensions, VisibilityExtensionErrors uint64
	OldestAge                                                                             time.Duration
	LastHeartbeat                                                                         time.Time
}

func (m *WorkerMetrics) Snapshot() WorkerSnapshot {
	flight := m.inFlight.Load()
	if flight < 0 {
		flight = 0
	}
	return WorkerSnapshot{Name: m.name, Started: m.started.Load(), Completed: m.completed.Load(), Failed: m.failed.Load(), InFlight: uint64(flight), VisibilityExtensions: m.visibilityExtend.Load(), VisibilityExtensionErrors: m.visibilityErrors.Load(), OldestAge: time.Duration(m.oldestAgeNanos.Load()), LastHeartbeat: time.Unix(0, m.lastHeartbeat.Load()).UTC()}
}

func HTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writer := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(writer, r)
		httpRequests.Add(1)
		if writer.status >= http.StatusInternalServerError {
			httpServerErrors.Add(1)
		}
	})
}

func Handler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = fmt.Fprintf(w, "sender_api_http_requests_total %d\n", httpRequests.Load())
	_, _ = fmt.Fprintf(w, "sender_api_http_server_errors_total %d\n", httpServerErrors.Load())
	workerRegistry.RLock()
	defer workerRegistry.RUnlock()
	for _, m := range workerRegistry.items {
		s := m.Snapshot()
		prefix := "sender_api_worker_" + s.Name + "_"
		_, _ = fmt.Fprintf(w, "%sstarted_total %d\n%scompleted_total %d\n%sfailed_total %d\n%sin_flight %d\n%soldest_age_seconds %f\n%svisibility_extensions_total %d\n%svisibility_extension_errors_total %d\n%sheartbeat_timestamp_seconds %d\n", prefix, s.Started, prefix, s.Completed, prefix, s.Failed, prefix, s.InFlight, prefix, s.OldestAge.Seconds(), prefix, s.VisibilityExtensions, prefix, s.VisibilityExtensionErrors, prefix, s.LastHeartbeat.Unix())
	}
	customRegistry.RLock()
	names := make([]string, 0, len(customRegistry.counters)+len(customRegistry.gauges))
	for name := range customRegistry.counters {
		names = append(names, name)
	}
	for name := range customRegistry.gauges {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if counter := customRegistry.counters[name]; counter != nil {
			_, _ = fmt.Fprintf(w, "%s %d\n", name, counter.Load())
			continue
		}
		if gauge := customRegistry.gauges[name]; gauge != nil {
			_, _ = fmt.Fprintf(w, "%s %d\n", name, gauge.Load())
		}
	}
	customRegistry.RUnlock()
}

type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.status = status
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(body []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}
