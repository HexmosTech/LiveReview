-- migrate:up

-- Separates /chat and /chat-debug into distinct conversation lists. Both
-- pages create conversations through the same chat_conversations table
-- today, so a thread started from one shows up in the other's sidebar too.
-- surface records which UI surface a conversation was started from, so each
-- page's sidebar can filter to just its own list.

ALTER TABLE chat_conversations
    ADD COLUMN surface VARCHAR(20) NOT NULL DEFAULT 'chat'
    CHECK (surface IN ('chat', 'chat_debug'));

CREATE INDEX idx_chat_conversations_user_active_surface
    ON chat_conversations(user_id, org_id, surface, updated_at DESC)
    WHERE deleted_at IS NULL;

-- migrate:down

DROP INDEX IF EXISTS idx_chat_conversations_user_active_surface;
ALTER TABLE chat_conversations DROP COLUMN IF EXISTS surface;
