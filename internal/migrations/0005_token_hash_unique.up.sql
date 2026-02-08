CREATE UNIQUE INDEX IF NOT EXISTS team_tokens_token_hash_uq
  ON team_tokens(token_hash);

CREATE UNIQUE INDEX IF NOT EXISTS runners_token_hash_uq
  ON runners(token_hash);
