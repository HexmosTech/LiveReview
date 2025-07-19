-- Generated schema based on migrations
-- This can be used for LLM context

CREATE TABLE instance_details (
  id SERIAL PRIMARY KEY,
  livereview_prod_url TEXT NOT NULL,
  admin_password TEXT NOT NULL,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP NOT NULL,
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP NOT NULL
);
