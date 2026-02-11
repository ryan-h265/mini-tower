ALTER TABLE team_tokens
  ADD COLUMN role TEXT NOT NULL DEFAULT 'admin' CHECK (role IN ('admin', 'member'));
