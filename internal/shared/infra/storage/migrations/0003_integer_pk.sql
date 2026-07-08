-- Rebuild memories with an explicit INTEGER PRIMARY KEY (pk).
--
-- Why: an implicit rowid may be renumbered by VACUUM — a hazard the FTS
-- external-content index (content_rowid) has silently depended on since
-- 0001. Declaring the alias makes rowids permanent by schema, and gives the
-- keyword mirror an integer join key: probing and grouping 8-byte ints
-- instead of 36-byte uuid strings is ~30% faster at 100k rows. pk is copied
-- from the old implicit rowid, so existing FTS postings stay valid with no
-- reindex. The uuid id column remains the public identity everywhere.

CREATE TABLE memories_new (
  pk               integer PRIMARY KEY,
  id               text NOT NULL UNIQUE,
  content          text NOT NULL,
  summary          text NOT NULL,
  kind             text NOT NULL,
  keywords         text NOT NULL,  -- normalized, space-separated, padded: ' k1 k2 '
  source           text NOT NULL,
  ttl_hours        integer NOT NULL DEFAULT 0,
  expires_at       text,           -- NULL = never; precomputed for SQL filtering
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
SELECT rowid, id, content, summary, kind, keywords, source, ttl_hours,
       expires_at, created_at, superseded_by, votes_up, votes_down,
       access_count, last_accessed_at
  FROM memories;

DROP TABLE memories; -- takes the fts triggers and indexes with it
ALTER TABLE memories_new RENAME TO memories;

CREATE INDEX memories_created_at_idx ON memories (created_at);
CREATE INDEX memories_kind_idx ON memories (kind);

-- FTS triggers verbatim from 0001 — new.rowid/old.rowid now alias pk
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

-- Keyword mirror keyed by pk (repository-maintained, see 0002 for rationale)
DROP TABLE memory_keywords;
CREATE TABLE memory_keywords (
  keyword   text NOT NULL,
  memory_pk integer NOT NULL,
  PRIMARY KEY (keyword, memory_pk)
) WITHOUT ROWID;

CREATE INDEX memory_keywords_pk_idx ON memory_keywords (memory_pk);

WITH RECURSIVE split (pk, keyword, rest) AS (
  SELECT pk, '', trim(keywords) || ' ' FROM memories
  UNION ALL
  SELECT pk,
         substr(rest, 1, instr(rest, ' ') - 1),
         substr(rest, instr(rest, ' ') + 1)
    FROM split
   WHERE rest <> ''
)
INSERT INTO memory_keywords (keyword, memory_pk)
SELECT DISTINCT keyword, pk FROM split WHERE keyword <> '';
