-- Model aliases map bare names to provider/model targets.
CREATE TABLE IF NOT EXISTS model_aliases (
    alias  TEXT PRIMARY KEY,
    target TEXT NOT NULL
);
