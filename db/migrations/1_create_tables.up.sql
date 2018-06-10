CREATE TABLE IF NOT EXISTS games (
  id serial PRIMARY KEY NOT NULL,
  slug text,
  created_at timestamp DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS tags (
  game_id bigint,
  key text,
  value text,
  created_at timestamp DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS moves (
  game_id bigint,
  player int,
  turn int,
  text text,
  created_at timestamp DEFAULT CURRENT_TIMESTAMP
);
