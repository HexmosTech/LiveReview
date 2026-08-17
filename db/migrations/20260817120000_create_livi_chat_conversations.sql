-- migrate:up

-- Livi chat conversations, messages, and chart artifacts. Previously chat was
-- fully ephemeral: the client held history in memory and it was lost on
-- reload. This gives conversations a durable, searchable home per user, and
-- lets charts be re-rendered on demand later without re-running the LLM.

CREATE TABLE chat_conversations (
    id BIGSERIAL PRIMARY KEY,
    org_id BIGINT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(200) NOT NULL DEFAULT 'New conversation',
    session_id VARCHAR(64),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,
    search_vector TSVECTOR
);

CREATE INDEX idx_chat_conversations_user_active
    ON chat_conversations(user_id, org_id, updated_at DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_chat_conversations_search
    ON chat_conversations USING GIN (search_vector);

CREATE TABLE chat_messages (
    id BIGSERIAL PRIMARY KEY,
    conversation_id BIGINT NOT NULL REFERENCES chat_conversations(id) ON DELETE CASCADE,
    role VARCHAR(20) NOT NULL CHECK (role IN ('user', 'assistant')),
    content TEXT NOT NULL,
    raw_history_entry JSONB NOT NULL,
    turn_seq INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    search_vector TSVECTOR,
    UNIQUE (conversation_id, turn_seq)
);

CREATE INDEX idx_chat_messages_conversation
    ON chat_messages(conversation_id, turn_seq);
CREATE INDEX idx_chat_messages_search
    ON chat_messages USING GIN (search_vector);

CREATE TABLE chat_charts (
    id BIGSERIAL PRIMARY KEY,
    message_id BIGINT NOT NULL REFERENCES chat_messages(id) ON DELETE CASCADE,
    title VARCHAR(300),
    description TEXT,
    query TEXT,
    time_range VARCHAR(100),
    granularity VARCHAR(50),
    triggering_user_message TEXT NOT NULL,
    vega_spec JSONB NOT NULL,
    raw_llm_output TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_chat_charts_message ON chat_charts(message_id);

CREATE FUNCTION chat_conversations_search_trigger() RETURNS trigger AS $$
BEGIN
  NEW.search_vector := to_tsvector('english', COALESCE(NEW.title, ''));
  RETURN NEW;
END
$$ LANGUAGE plpgsql;

CREATE TRIGGER chat_conversations_search_update
  BEFORE INSERT OR UPDATE OF title ON chat_conversations
  FOR EACH ROW EXECUTE FUNCTION chat_conversations_search_trigger();

CREATE FUNCTION chat_messages_search_trigger() RETURNS trigger AS $$
BEGIN
  NEW.search_vector := to_tsvector('english', COALESCE(NEW.content, ''));
  RETURN NEW;
END
$$ LANGUAGE plpgsql;

CREATE TRIGGER chat_messages_search_update
  BEFORE INSERT OR UPDATE OF content ON chat_messages
  FOR EACH ROW EXECUTE FUNCTION chat_messages_search_trigger();

-- migrate:down

DROP TRIGGER IF EXISTS chat_messages_search_update ON chat_messages;
DROP FUNCTION IF EXISTS chat_messages_search_trigger();
DROP TRIGGER IF EXISTS chat_conversations_search_update ON chat_conversations;
DROP FUNCTION IF EXISTS chat_conversations_search_trigger();
DROP TABLE IF EXISTS chat_charts;
DROP TABLE IF EXISTS chat_messages;
DROP TABLE IF EXISTS chat_conversations;
