BEGIN;
ALTER TABLE playlist_metadata
    ALTER COLUMN created_at SET DATA TYPE TIMESTAMP WITH TIME ZONE,
    ALTER COLUMN updated_at SET DATA TYPE TIMESTAMP WITH TIME ZONE,
    ALTER COLUMN deleted_at SET DATA TYPE TIMESTAMP WITH TIME ZONE;
COMMIT;
