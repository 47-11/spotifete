ALTER TABLE song_requests
    ADD COLUMN status VARCHAR NOT NULL DEFAULT 'IN_QUEUE';
