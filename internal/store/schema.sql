CREATE TABLE days (
  id TEXT NOT NULL PRIMARY KEY,
  date TEXT NOT NULL UNIQUE,
  timezone TEXT NOT NULL,
  created_at_utc TEXT NOT NULL,
  updated_at_utc TEXT
);

CREATE TABLE blocks (
  id TEXT NOT NULL PRIMARY KEY,
  title TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('todo', 'in_progress', 'paused', 'done', 'skipped')),

  day_id TEXT NOT NULL REFERENCES days(id),
  copied_from_id TEXT REFERENCES blocks(id),

  archived INTEGER NOT NULL DEFAULT 0 CHECK (archived IN (0, 1)),

  planned_start_utc TEXT NOT NULL,
  planned_end_utc TEXT NOT NULL,
  actual_start_utc TEXT,
  actual_end_utc TEXT,
  created_at_utc TEXT NOT NULL,
  updated_at_utc TEXT
);

CREATE TABLE block_events (
  seq INTEGER PRIMARY KEY AUTOINCREMENT,
  block_id TEXT NOT NULL REFERENCES blocks(id),
  from_status TEXT NOT NULL CHECK ( from_status IN ('todo', 'in_progress', 'paused', 'done', 'skipped') ),
  to_status TEXT NOT NULL CHECK ( to_status IN ('todo', 'in_progress', 'paused', 'done', 'skipped') ),
  occurred_at_utc TEXT NOT NULL,
  reason TEXT NOT NULL CHECK ( reason IN ('user', 'auto_pause', 'carryover', 'reopen', 'extend'))
);

CREATE INDEX blocks_by_day_and_start ON blocks (day_id, planned_start_utc);
CREATE INDEX events_by_block_and_sequence ON block_events (block_id, seq);
