CREATE TABLE memories (
  id               text PRIMARY KEY,
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

CREATE INDEX memories_created_at_idx ON memories (created_at);
CREATE INDEX memories_kind_idx ON memories (kind);

-- Full-text index over what agents search by. Maintained by TRIGGERS, not a
-- projector: the database is multi-process (MCP server + CLI one-shots), and
-- in-process events cannot index another process's writes.
CREATE VIRTUAL TABLE memories_fts USING fts5(
  summary, content, keywords,
  content='memories', content_rowid='rowid'
);

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
