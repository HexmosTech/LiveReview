-- migrate:up

-- Plain to_tsquery/websearch_to_tsquery only match whole stemmed words, so
-- searching "rev" finds nothing even though "review" is right there. Prefix
-- matching (word:*) fixes that; pg_trgm's similarity() adds typo-tolerant
-- fuzzy matching on top, scoped to the (short, cheap-to-index) title column.
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX idx_chat_conversations_title_trgm
    ON chat_conversations USING GIN (title gin_trgm_ops);

-- migrate:down

DROP INDEX IF EXISTS idx_chat_conversations_title_trgm;
DROP EXTENSION IF EXISTS pg_trgm;
