-- Every memory expires: "ttl_hours = 0 means never" is gone.
--
-- Why: a memory nobody bounded outlives its usefulness, keeps ranking, and
-- misleads the next agent. The writer now picks a lifetime, so expires_at is
-- NOT NULL by schema rather than by convention (see AGENTS.md: invariants
-- live in the aggregate — this one is cheap enough to also enforce here).
--
-- Legacy rows (ttl_hours = 0, expires_at NULL) get one month from the
-- migration moment. Anchoring on created_at instead would have expired every
-- memory older than a month the instant the new binary opened the file.

UPDATE memories
   SET ttl_hours = max(1, CAST((julianday('now', '+30 days') - julianday(created_at)) * 24 AS integer))
 WHERE expires_at IS NULL;

UPDATE memories
   SET expires_at = strftime('%Y-%m-%dT%H:%M:%SZ', created_at, '+' || ttl_hours || ' hours')
 WHERE expires_at IS NULL;

-- Rebuild to tighten the columns (SQLite cannot ALTER a column). Same dance
-- as 0003: pk is copied verbatim, so FTS postings and the keyword mirror stay
-- valid and need no reindex. DROP TABLE fires no row triggers.

CREATE TABLE memories_new (
  pk               integer PRIMARY KEY,
  id               text NOT NULL UNIQUE,
  content          text NOT NULL,
  summary          text NOT NULL,
  kind             text NOT NULL,
  keywords         text NOT NULL,  -- normalized, space-separated, padded: ' k1 k2 '
  source           text NOT NULL,
  ttl_hours        integer NOT NULL CHECK (ttl_hours > 0),
  expires_at       text NOT NULL,  -- created_at + ttl_hours; precomputed for SQL filtering
  created_at       text NOT NULL,
  superseded_by    text,           -- NULL = live
  votes_up         integer NOT NULL DEFAULT 0,
  votes_down       integer NOT NULL DEFAULT 0,
  access_count     integer NOT NULL DEFAULT 0,
  last_accessed_at text
);

INSERT INTO memories_new (pk, id, content, summary, kind, keywords, source, ttl_hours,
                          expires_at, created_at, superseded_by, votes_up, votes_down,
                          access_count, last_accessed_at)
SELECT pk, id, content, summary, kind, keywords, source, ttl_hours,
       expires_at, created_at, superseded_by, votes_up, votes_down,
       access_count, last_accessed_at
  FROM memories;

DROP TABLE memories; -- takes the fts triggers and indexes with it
ALTER TABLE memories_new RENAME TO memories;

CREATE INDEX memories_created_at_idx ON memories (created_at);
CREATE INDEX memories_kind_idx ON memories (kind);

-- FTS triggers verbatim from 0003 — new.rowid/old.rowid alias pk
CREATE TRIGGER memories_fts_insert AFTER INSERT ON memories BEGIN
  INSERT INTO memories_fts(rowid, summary, content, keywords)
  VALUES (new.rowid, new.summary, new.content, new.keywords);
END;

CREATE TRIGGER memories_fts_delete AFTER DELETE ON memories BEGIN
  INSERT INTO memories_fts(memories_fts, rowid, summary, content, keywords)
  VALUES ('delete', old.rowid, old.summary, old.content, old.keywords);
END;

CREATE TRIGGER memories_fts_update AFTER UPDATE ON memories BEGIN
  INSERT INTO memories_fts(memories_fts, rowid, summary, content, keywords)
  VALUES ('delete', old.rowid, old.summary, old.content, old.keywords);
  INSERT INTO memories_fts(rowid, summary, content, keywords)
  VALUES (new.rowid, new.summary, new.content, new.keywords);
END;
