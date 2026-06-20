-- Resource monitor: one row per sampler tick (~15s), pruned after 7 days.
-- Timestamps are RFC3339 TEXT for engine portability, matching other tables.
CREATE TABLE IF NOT EXISTS resource_samples (
    id                    INTEGER PRIMARY KEY,
    tenant_id             TEXT NOT NULL,
    created_at            TEXT NOT NULL,

    goroutines            INTEGER NOT NULL,
    heap_alloc_bytes      INTEGER NOT NULL,
    heap_sys_bytes        INTEGER NOT NULL,
    gc_pause_ns           INTEGER NOT NULL,
    next_gc_bytes         INTEGER NOT NULL,
    num_gc                INTEGER NOT NULL,

    proc_cpu_percent      REAL NOT NULL,
    proc_rss_bytes        INTEGER NOT NULL,
    proc_threads          INTEGER NOT NULL,
    proc_open_fds         INTEGER,

    host_cpu_percent      REAL NOT NULL,
    host_mem_used_bytes   INTEGER NOT NULL,
    host_mem_total_bytes  INTEGER NOT NULL,
    host_disk_used_bytes  INTEGER NOT NULL,
    host_disk_total_bytes INTEGER NOT NULL,
    host_net_sent_bytes   INTEGER NOT NULL,
    host_net_recv_bytes   INTEGER NOT NULL,
    host_load1            REAL,
    host_load5            REAL,
    host_load15           REAL,

    inflight_requests     INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_resource_samples_created_at ON resource_samples(created_at);
