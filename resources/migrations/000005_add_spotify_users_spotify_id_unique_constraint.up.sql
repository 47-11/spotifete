BEGIN;
ALTER TABLE spotify_users
    ADD CONSTRAINT spotify_id_unique UNIQUE(spotify_id);
COMMIT;
