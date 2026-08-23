-- migrate:up

-- Adds "context" - the specific person/repository names a chart or export is
-- scoped to (e.g. {"Ganesh Kumar", "Shrijith Sharma"}, {"git-lrc"}, or an
-- empty array for organization-wide) - to the two tables added in
-- 20260817120000 and 20260820141616.

ALTER TABLE chat_charts ADD COLUMN context TEXT[];
ALTER TABLE chat_files ADD COLUMN context TEXT[];

-- migrate:down

ALTER TABLE chat_charts DROP COLUMN IF EXISTS context;
ALTER TABLE chat_files DROP COLUMN IF EXISTS context;
