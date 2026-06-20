package gateway

import (
	"context"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/store"
)

const (
	historySize    = 60 // 5 min at 5s interval
	sampleInterval = 5 * time.Second
	cpuSpikeThresh = 80.0
	memSpikeThresh = 85.0

	// DB persistence settings for resource_samples.
	resourceDBInterval = 15 * time.Second
	resourceRetention  = 7 * 24 * time.Hour // 7 days
)

// SystemSample holds one point-in-time snapshot for the history ring buffer.
type SystemSample struct {
	Timestamp   int64   `json:"ts"`
	CPUPct      float64 `json:"cpu_pct"`
	MemUsedPct  float64 `json:"mem_pct"`
	Goroutines  int     `json:"goroutines"`
	HeapAllocMB float64 `json:"heap_mb"`
	IsCPUSpike  bool    `json:"cpu_spike,omitempty"`
	IsMemSpike  bool    `json:"mem_spike,omitempty"`

	// Process-level metrics (nexusrouter's own resource usage).
	ProcCPUPct   float64 `json:"proc_cpu_pct,omitempty"`
	ProcRSSMB    float64 `json:"proc_rss_mb,omitempty"`
	ProcThreads  int32   `json:"proc_threads,omitempty"`
	ProcOpenFDs  int32   `json:"proc_open_fds,omitempty"`
}

// SystemSnapshot is the detailed real-time payload for the current moment.
type SystemSnapshot struct {
	// CPU
	CPUPct     float64   `json:"cpu_pct"`
	CPUPerCore []float64 `json:"cpu_per_core"`

	// Memory
	MemTotalMB     uint64  `json:"mem_total_mb"`
	MemUsedMB      uint64  `json:"mem_used_mb"`
	MemAvailableMB uint64  `json:"mem_available_mb"`
	MemUsedPct     float64 `json:"mem_pct"`

	// Disk
	DiskTotalGB float64 `json:"disk_total_gb"`
	DiskUsedGB  float64 `json:"disk_used_gb"`
	DiskFreeGB  float64 `json:"disk_free_gb"`
	DiskUsedPct float64 `json:"disk_pct"`

	// Go runtime
	Goroutines  int     `json:"goroutines"`
	HeapAllocMB float64 `json:"heap_alloc_mb"`
	HeapSysMB   float64 `json:"heap_sys_mb"`
	HeapInuseMB float64 `json:"heap_inuse_mb"`
	HeapIdleMB  float64 `json:"heap_idle_mb"`

	// GC
	GCPauseTotalMs float64 `json:"gc_pause_total_ms"`
	GCPauseLastMs  float64 `json:"gc_pause_last_ms"`
	GCCycles       uint32  `json:"gc_cycles"`

	// Process
	OpenFDs int    `json:"open_fds"`
	UptimeS int64  `json:"uptime_s"`
	PID     int    `json:"pid"`
	Host    string `json:"host"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`

	// Network
	NetConns int `json:"net_conns"`

	// Process-level metrics (nexusrouter's own resource usage).
	ProcCPUPct   float64 `json:"proc_cpu_pct"`
	ProcRSSMB    float64 `json:"proc_rss_mb"`
	ProcThreads  int32   `json:"proc_threads"`
	ProcOpenFDs  int32   `json:"proc_open_fds"`
}

// systemHistory holds a circular buffer of samples.
type systemHistory struct {
	mu      sync.RWMutex
	buf     [historySize]SystemSample
	head    int
	count   int
	started time.Time
}

func newSystemHistory() *systemHistory {
	return &systemHistory{started: time.Now()}
}

func (h *systemHistory) push(s SystemSample) {
	h.mu.Lock()
	h.buf[h.head] = s
	h.head = (h.head + 1) % historySize
	if h.count < historySize {
		h.count++
	}
	h.mu.Unlock()
}

func (h *systemHistory) samples() []SystemSample {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]SystemSample, h.count)
	for i := 0; i < h.count; i++ {
		idx := (h.head - h.count + i + historySize) % historySize
		out[i] = h.buf[idx]
	}
	return out
}

// Global; shared between the background collector and HTTP handlers.
var sysHistory = newSystemHistory()

// startSystemCollector launches a background goroutine that samples system
// metrics every 5 seconds into the ring buffer. If resRepo is non-nil, a
// second goroutine writes a full resource sample to the DB every 15 seconds
// and prunes rows older than 7 days.
func startSystemCollector(resRepo *store.ResourceRepo) {
	// Take one immediate sample so the history isn't empty on first request.
	sysHistory.push(collectSample())

	go func() {
		ticker := time.NewTicker(sampleInterval)
		defer ticker.Stop()
		for range ticker.C {
			sysHistory.push(collectSample())
		}
	}()

	// DB persistence goroutine: writes a resource_samples row every 15s
	// and prunes rows older than the retention window.
	if resRepo != nil {
		go func() {
			ticker := time.NewTicker(resourceDBInterval)
			defer ticker.Stop()
			pruneTicker := time.NewTicker(1 * time.Hour)
			defer pruneTicker.Stop()

			for {
				select {
				case <-ticker.C:
					s := collectResourceSample()
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					_ = resRepo.InsertResourceSample(ctx, s)
					cancel()
				case <-pruneTicker.C:
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					_ = resRepo.PruneResourceSamples(ctx, resourceRetention)
					cancel()
				}
			}
		}()
	}
}

// collectResourceSample gathers a full resource snapshot for DB persistence,
// including both process-level and host-level metrics that the migration table
// expects.
func collectResourceSample() store.ResourceSample {
	s := store.ResourceSample{
		TenantID:  "default",
		CreatedAt: time.Now(),
	}

	// Go runtime
	s.Goroutines = int64(runtime.NumGoroutine())
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	s.HeapAllocBytes = int64(m.HeapAlloc)
	s.HeapSysBytes = int64(m.HeapSys)
	s.GCPauseNS = int64(m.PauseTotalNs)
	s.NextGCBytes = int64(m.NextGC)
	s.NumGC = int64(m.NumGC)

	// Process-level metrics
	pid := int32(os.Getpid())
	if p, err := process.NewProcess(pid); err == nil {
		if pct, err := p.Percent(0); err == nil {
			s.ProcCPUPercent = pct
		}
		if mi, err := p.MemoryInfo(); err == nil && mi != nil {
			s.ProcRSSBytes = int64(mi.RSS)
		}
		if t, err := p.NumThreads(); err == nil {
			s.ProcThreads = int64(t)
		}
		if fds, err := p.NumFDs(); err == nil {
			v := int64(fds)
			s.ProcOpenFDs = &v
		}
	}

	// Host CPU
	if pcts, err := cpu.Percent(0, false); err == nil && len(pcts) > 0 {
		s.HostCPUPercent = pcts[0]
	}

	// Host memory
	if vm, err := mem.VirtualMemory(); err == nil {
		s.HostMemUsedBytes = int64(vm.Used)
		s.HostMemTotalBytes = int64(vm.Total)
	}

	// Host disk (root partition)
	if usage, err := disk.Usage("/"); err == nil {
		s.HostDiskUsedBytes = int64(usage.Used)
		s.HostDiskTotalBytes = int64(usage.Total)
	}

	// Host network I/O (cumulative counters)
	if counters, err := net.IOCounters(false); err == nil && len(counters) > 0 {
		s.HostNetSentBytes = int64(counters[0].BytesSent)
		s.HostNetRecvBytes = int64(counters[0].BytesRecv)
	}

	// Host load averages
	if avg, err := load.Avg(); err == nil {
		l1 := avg.Load1
		l5 := avg.Load5
		l15 := avg.Load15
		s.HostLoad1 = &l1
		s.HostLoad5 = &l5
		s.HostLoad15 = &l15
	}

	return s
}

func collectSample() SystemSample {
	s := SystemSample{Timestamp: time.Now().Unix()}

	if pcts, err := cpu.Percent(0, false); err == nil && len(pcts) > 0 {
		s.CPUPct = pcts[0]
	}
	if vm, err := mem.VirtualMemory(); err == nil {
		s.MemUsedPct = vm.UsedPercent
	}

	s.Goroutines = runtime.NumGoroutine()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	s.HeapAllocMB = float64(m.HeapAlloc) / (1024 * 1024)

	// Process-level metrics: this application's own CPU and memory.
	pid := int32(os.Getpid())
	if p, err := process.NewProcess(pid); err == nil {
		if pct, err := p.Percent(0); err == nil {
			s.ProcCPUPct = pct
		}
		if mi, err := p.MemoryInfo(); err == nil && mi != nil {
			s.ProcRSSMB = float64(mi.RSS) / (1024 * 1024)
		}
		if t, err := p.NumThreads(); err == nil {
			s.ProcThreads = int32(t)
		}
		if fds, err := p.NumFDs(); err == nil {
			s.ProcOpenFDs = int32(fds)
		}
	}

	s.IsCPUSpike = s.CPUPct >= cpuSpikeThresh
	s.IsMemSpike = s.MemUsedPct >= memSpikeThresh

	return s
}

// ---- HTTP handlers ----------------------------------------------------------

// adminSystem returns the current detailed system snapshot.
func (s *Server) adminSystem(w http.ResponseWriter, r *http.Request) {
	snap := collectFullSnapshot()
	writeJSON(w, http.StatusOK, snap)
}

// adminSystemResourceHistory returns time-bucketed resource samples from the
// database for long-term trend charts. Query params: ?hours=24&interval=5m
func (s *Server) adminSystemResourceHistory(w http.ResponseWriter, r *http.Request) {
	if s.resources == nil {
		writeJSON(w, http.StatusOK, []store.ResourceBucket{})
		return
	}
	hours := 24
	if h := r.URL.Query().Get("hours"); h != "" {
		if v, err := time.ParseDuration(h + "h"); err == nil && v > 0 {
			hours = int(v.Hours())
		}
	}
	interval := 5 * time.Minute
	if iv := r.URL.Query().Get("interval"); iv != "" {
		if d, err := time.ParseDuration(iv); err == nil && d >= time.Minute {
			interval = d
		}
	}
	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	buckets, err := s.resources.ResourceBuckets(r.Context(), since, interval)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load resource history")
		return
	}
	if buckets == nil {
		buckets = []store.ResourceBucket{}
	}
	writeJSON(w, http.StatusOK, buckets)
}

// adminSystemHistory returns the historical ring buffer of samples.
func (s *Server) adminSystemHistory(w http.ResponseWriter, r *http.Request) {
	type response struct {
		Interval int             `json:"interval_sec"`
		MaxSize  int             `json:"max_size"`
		Spikes   []SystemSample  `json:"spikes"`
		Samples  []SystemSample  `json:"samples"`
	}
	samples := sysHistory.samples()

	// Extract spikes for quick overview
	spikes := make([]SystemSample, 0)
	for _, s := range samples {
		if s.IsCPUSpike || s.IsMemSpike {
			spikes = append(spikes, s)
		}
	}

	writeJSON(w, http.StatusOK, response{
		Interval: int(sampleInterval.Seconds()),
		MaxSize:  historySize,
		Spikes:   spikes,
		Samples:  samples,
	})
}

func collectFullSnapshot() SystemSnapshot {
	s := SystemSnapshot{}

	// CPU overall
	if pcts, err := cpu.Percent(0, false); err == nil && len(pcts) > 0 {
		s.CPUPct = pcts[0]
	}
	// CPU per-core
	if pcts, err := cpu.Percent(0, true); err == nil {
		s.CPUPerCore = pcts
	}

	// Memory
	if vm, err := mem.VirtualMemory(); err == nil {
		s.MemTotalMB = vm.Total / (1024 * 1024)
		s.MemUsedMB = vm.Used / (1024 * 1024)
		s.MemAvailableMB = vm.Available / (1024 * 1024)
		s.MemUsedPct = vm.UsedPercent
	}

	// Disk (root partition)
	if usage, err := disk.Usage("/"); err == nil {
		s.DiskTotalGB = float64(usage.Total) / (1024 * 1024 * 1024)
		s.DiskUsedGB = float64(usage.Used) / (1024 * 1024 * 1024)
		s.DiskFreeGB = float64(usage.Free) / (1024 * 1024 * 1024)
		s.DiskUsedPct = usage.UsedPercent
	}

	// Go runtime
	s.Goroutines = runtime.NumGoroutine()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	s.HeapAllocMB = float64(m.HeapAlloc) / (1024 * 1024)
	s.HeapSysMB = float64(m.HeapSys) / (1024 * 1024)
	s.HeapInuseMB = float64(m.HeapInuse) / (1024 * 1024)
	s.HeapIdleMB = float64(m.HeapIdle) / (1024 * 1024)

	// GC
	s.GCPauseTotalMs = float64(m.PauseTotalNs) / 1e6
	if m.NumGC > 0 {
		lastIdx := (m.NumGC + 255) % 256
		s.GCPauseLastMs = float64(m.PauseNs[lastIdx]) / 1e6
	}
	s.GCCycles = m.NumGC

	// Process info
	pid := int32(os.Getpid())
	s.PID = int(pid)
	if p, err := process.NewProcess(pid); err == nil {
		if fds, err := p.NumFDs(); err == nil {
			s.OpenFDs = int(fds)
		}
		if pct, err := p.Percent(0); err == nil {
			s.ProcCPUPct = pct
		}
		if mi, err := p.MemoryInfo(); err == nil && mi != nil {
			s.ProcRSSMB = float64(mi.RSS) / (1024 * 1024)
		}
		if t, err := p.NumThreads(); err == nil {
			s.ProcThreads = int32(t)
		}
		s.ProcOpenFDs = int32(s.OpenFDs)
	}

	// Uptime
	s.UptimeS = int64(time.Since(sysHistory.started).Seconds())

	// Host info
	if hi, err := host.Info(); err == nil {
		s.Host = hi.Hostname
		s.OS = hi.Platform + " " + hi.PlatformVersion
		s.Arch = hi.KernelArch
	}

	// Network connections count — stored separately so it no longer
	// clobbers the process-level OpenFDs field.
	if conns, err := net.Connections("all"); err == nil {
		s.NetConns = len(conns)
	}

	return s
}