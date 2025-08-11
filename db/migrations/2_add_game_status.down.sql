-- Drop indexes
DROP INDEX IF EXISTS idx_games_status;
DROP INDEX IF EXISTS idx_moves_game_turn;

-- Remove game status tracking columns
ALTER TABLE games DROP COLUMN IF EXISTS status;
ALTER TABLE games DROP COLUMN IF EXISTS winner;
ALTER TABLE games DROP COLUMN IF EXISTS current_player;
ALTER TABLE games DROP COLUMN IF EXISTS current_turn;