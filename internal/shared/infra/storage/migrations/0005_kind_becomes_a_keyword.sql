-- kind stops being a column and becomes a keyword convention.
--
-- Why: kind was required on every write, closed to six values, and read by
-- nothing that matters — it is absent from the ranking formula, so it could
-- only ever narrow a search. Keywords already do that job, and better: free
-- form, multi-valued, indexed, and full-text searchable. Every kind is folded
-- into a `kind:<value>` keyword first, so no classification is lost and the
-- old `--kind decision` filter survives as the keyword `kind:decision`.

UPDATE memories
   SET keywords = keywords || 'kind:' || kind || ' '
 WHERE kind <> '' AND keywords NOT LIKE '% kind:%';

-- keywords are padded ' k1 k2 ', so the append above lands after the trailing
-- space and the padding still holds. FTS reindexes itself through the update
-- trigger; the mirror is repository-maintained, so it is filled here.
INSERT OR IGNORE INTO memory_keywords (keyword, memory_pk)
SELECT 'kind:' || kind, pk FROM memories WHERE kind <> '';

-- Rebuild without the column (SQLite cannot DROP a column referenced by an
-- index). Same dance as 0003/0004: pk copied verbatim, so FTS postings and
-- the keyword mirror stay valid; DROP TABLE fires no row triggers.

CREATE TABLE memories_new (
  pk               integer PRIMARY KEY,
  id               text NOT NULL UNIQUE,
  content          text NOT NULL,
  summary          text NOT NULL,
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

INSERT INTO memories_new (pk, id, content, summary, keywords, source, ttl_hours,
                          expires_at, created_at, superseded_by, votes_up, votes_down,
                          access_count, last_accessed_at)
SELECT pk, id, content, summary, keywords, source, ttl_hours,
       expires_at, created_at, superseded_by, votes_up, votes_down,
       access_count, last_accessed_at
  FROM memories;

DROP TABLE memories; -- takes the fts triggers and indexes with it
ALTER TABLE memories_new RENAME TO memories;

CREATE INDEX memories_created_at_idx ON memories (created_at);

-- FTS triggers verbatim from 0004 — new.rowid/old.rowid alias pk
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
