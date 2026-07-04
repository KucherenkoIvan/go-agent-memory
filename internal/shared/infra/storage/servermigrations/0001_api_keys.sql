-- Hosted-mode control plane: spaces and the API keys that unlock them.
-- Lives in its own keys.db — space memory files stay byte-identical to a
-- local memory.db (migrate-to-local by copying the file).

CREATE TABLE spaces (
  name       text PRIMARY KEY, -- validated [a-z0-9-]+ at key creation
  created_at text NOT NULL
);

CREATE TABLE api_keys (
  id         text PRIMARY KEY,
  name       text NOT NULL,             -- key label; becomes memory `source`
  space      text NOT NULL REFERENCES spaces(name),
  token_hash text NOT NULL UNIQUE,      -- hex(sha256(raw token)); raw never stored
  prefix     text NOT NULL,             -- display handle, e.g. agm_a1b2c3d4
  created_at text NOT NULL,
  revoked_at text                       -- NULL = active
);

CREATE INDEX api_keys_space_idx ON api_keys (space);
