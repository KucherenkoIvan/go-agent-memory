-- Keyword index: exact (keyword, memory_id) pairs so search matches and
-- counts keywords by index probe instead of scanning every row's padded
-- keywords string (~3.5x faster at 100k rows, near-flat tail).
--
-- memories.keywords stays the source of truth — it feeds the FTS index and
-- the read models; this table is derived and rebuildable from it. Unlike
-- FTS it is maintained by the repository inside the write transaction, not
-- by triggers: keywords arrive in Go as an already-normalized slice, while
-- a trigger would have to re-split the packed string in SQL. Every writer
-- goes through the repository; a writer running a pre-0002 binary would
-- skip this table, so restart long-lived processes after upgrading.
CREATE TABLE memory_keywords (
  keyword   text NOT NULL,
  memory_id text NOT NULL,
  PRIMARY KEY (keyword, memory_id)
) WITHOUT ROWID;

-- the delete path removes links by memory
CREATE INDEX memory_keywords_memory_idx ON memory_keywords (memory_id);

-- backfill from the packed ' k1 k2 ' strings
WITH RECURSIVE split (memory_id, keyword, rest) AS (
  SELECT id, '', trim(keywords) || ' ' FROM memories
  UNION ALL
  SELECT memory_id,
         substr(rest, 1, instr(rest, ' ') - 1),
         substr(rest, instr(rest, ' ') + 1)
    FROM split
   WHERE rest <> ''
)
INSERT INTO memory_keywords (keyword, memory_id)
SELECT DISTINCT keyword, memory_id FROM split WHERE keyword <> '';
