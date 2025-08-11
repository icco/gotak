-- Add game status tracking columns
ALTER TABLE games ADD COLUMN status text DEFAULT 'active';
ALTER TABLE games ADD COLUMN winner int DEFAULT 0;
ALTER TABLE games ADD COLUMN current_player int DEFAULT 1;
ALTER TABLE games ADD COLUMN current_turn int DEFAULT 1;

-- Add index for better performance
CREATE INDEX idx_games_status ON games(status);
CREATE INDEX idx_moves_game_turn ON moves(game_id, turn);