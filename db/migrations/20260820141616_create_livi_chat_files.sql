-- migrate:up

-- Durable home for downloadable chat exports (CSV). Charts were given a home
-- in chat_charts (20260817120000); file exports previously lived only in an
-- in-memory map with a 10-minute TTL, so the "I've put the results in a file
-- you can download." message outlived the file it pointed to. Persisting them
-- here keeps the download available for as long as the conversation does.

CREATE TABLE chat_files (
    id BIGSERIAL PRIMARY KEY,
    message_id BIGINT NOT NULL REFERENCES chat_messages(id) ON DELETE CASCADE,
    kind VARCHAR(20) NOT NULL DEFAULT 'csv',
    filename TEXT NOT NULL,
    title TEXT,
    description TEXT,
    query TEXT,
    time_range VARCHAR(100),
    granularity VARCHAR(50),
    rows INTEGER,
    data BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_chat_files_message ON chat_files(message_id);

-- migrate:down

DROP TABLE IF EXISTS chat_files;