-- migrate:up

-- Temporary debug artifacts column for the multi-interpret analytics
-- pipeline. Stores the full intermediate representation of a count_query
-- turn (schema context, LLM response, interpretation results) as JSONB
-- for the /chat-debug page. Will be dropped when the chatbot works.

ALTER TABLE chat_messages ADD COLUMN debug_artifacts JSONB;

-- migrate:down

ALTER TABLE chat_messages DROP COLUMN IF EXISTS debug_artifacts;
