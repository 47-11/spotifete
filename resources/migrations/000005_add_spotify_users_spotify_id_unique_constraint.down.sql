BEGIN;
ALTER TABLE spotify_users
    DROP CONSTRAINT spotify_id_unique;
COMMIT;
