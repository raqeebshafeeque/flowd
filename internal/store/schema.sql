CREATE TABLE days (
 id TEXT NOT NULL PRIMARY KEY,
 date TEXT NOT NULL UNIQUE,
 timezone TEXT NOT NULL,
 created_at TEXT NOT NULL,
 updated_at TEXT
);
CREATE TABLE items (
 id TEXT NOT NULL PRIMARY KEY,
 day_id TEXT NOT NULL REFERENCES days(id),
 title TEXT NOT NULL,
 status TEXT NOT NULL CHECK (status IN ('todo','in_progress','paused','done','incomplete')),
 duration_minutes INTEGER NOT NULL CHECK (typeof(duration_minutes)='integer' AND duration_minutes>0),
 started_at TEXT,
 finished_at TEXT,
 closed_at TEXT,
 archived INTEGER NOT NULL DEFAULT 0 CHECK (archived IN (0,1)),
 created_at TEXT NOT NULL,
 updated_at TEXT,
 carried_from TEXT REFERENCES items(id),
 position INTEGER NOT NULL DEFAULT 0,
 corrected_at TEXT
);
CREATE TABLE item_events (
 seq INTEGER PRIMARY KEY AUTOINCREMENT,
 item_id TEXT NOT NULL REFERENCES items(id),
 from_status TEXT NOT NULL,
 to_status TEXT NOT NULL,
 occurred_at TEXT NOT NULL,
 recorded_at TEXT NOT NULL,
 reason TEXT NOT NULL CHECK (reason IN ('user','auto_pause','reopen','day_end')),
 before_json TEXT CHECK (before_json IS NULL OR json_valid(before_json)),
 after_json TEXT NOT NULL CHECK (json_valid(after_json))
);
CREATE INDEX items_by_day ON items(day_id, created_at);
CREATE INDEX unfinished_items ON items(day_id) WHERE archived=0 AND status IN ('todo','in_progress','paused');
CREATE INDEX events_by_item ON item_events(item_id, seq);
CREATE INDEX items_by_origin ON items(carried_from) WHERE carried_from IS NOT NULL;
CREATE UNIQUE INDEX items_in_order ON items(day_id, position) WHERE archived=0;
CREATE TRIGGER events_never_change BEFORE UPDATE ON item_events BEGIN SELECT RAISE(ABORT,'events cannot be changed'); END;
CREATE TRIGGER events_never_vanish BEFORE DELETE ON item_events BEGIN SELECT RAISE(ABORT,'events cannot be deleted'); END;
