SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: ai_comments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ai_comments (
    id bigint NOT NULL,
    review_id bigint NOT NULL,
    comment_type character varying(50) NOT NULL,
    content jsonb NOT NULL,
    file_path text,
    line_number integer,
    created_at timestamp with time zone DEFAULT now(),
    org_id bigint DEFAULT 1 NOT NULL,
    CONSTRAINT ai_comments_type_check CHECK (((comment_type)::text = ANY ((ARRAY['summary'::character varying, 'line_comment'::character varying, 'suggestion'::character varying, 'general'::character varying, 'file_comment'::character varying])::text[])))
);


--
-- Name: ai_comments_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.ai_comments_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: ai_comments_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.ai_comments_id_seq OWNED BY public.ai_comments.id;


--
-- Name: ai_connectors; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ai_connectors (
    id integer NOT NULL,
    provider_name character varying(64) NOT NULL,
    api_key text NOT NULL,
    display_order integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    connector_name character varying(128),
    base_url text,
    selected_model text,
    org_id bigint DEFAULT 1 NOT NULL
);


--
-- Name: COLUMN ai_connectors.connector_name; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.ai_connectors.connector_name IS 'A user-friendly name for the connector';


--
-- Name: ai_connectors_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.ai_connectors_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: ai_connectors_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.ai_connectors_id_seq OWNED BY public.ai_connectors.id;


--
-- Name: auth_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.auth_tokens (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    token_hash character varying(255) NOT NULL,
    token_type character varying(20) NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    last_used_at timestamp with time zone DEFAULT now() NOT NULL,
    user_agent text,
    ip_address inet,
    permissions jsonb DEFAULT '{}'::jsonb,
    rate_limit_requests_per_hour integer DEFAULT 1000,
    last_rate_limit_reset timestamp with time zone DEFAULT now(),
    requests_this_hour integer DEFAULT 0,
    revoked_at timestamp with time zone,
    is_active boolean DEFAULT true NOT NULL,
    CONSTRAINT auth_tokens_token_type_check CHECK (((token_type)::text = ANY ((ARRAY['session'::character varying, 'refresh'::character varying, 'api_key'::character varying])::text[])))
);


--
-- Name: auth_tokens_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.auth_tokens_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: auth_tokens_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.auth_tokens_id_seq OWNED BY public.auth_tokens.id;


--
-- Name: dashboard_cache; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.dashboard_cache (
    id integer DEFAULT 1 NOT NULL,
    data jsonb DEFAULT '{}'::jsonb NOT NULL,
    updated_at timestamp with time zone DEFAULT now(),
    created_at timestamp with time zone DEFAULT now(),
    org_id bigint DEFAULT 1 NOT NULL,
    CONSTRAINT single_dashboard_row CHECK ((id = 1))
);


--
-- Name: instance_details; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.instance_details (
    id integer NOT NULL,
    livereview_prod_url text,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    admin_password text NOT NULL
);


--
-- Name: instance_details_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.instance_details_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: instance_details_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.instance_details_id_seq OWNED BY public.instance_details.id;


--
-- Name: integration_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.integration_tokens (
    id bigint NOT NULL,
    provider text NOT NULL,
    provider_app_id text NOT NULL,
    access_token text NOT NULL,
    refresh_token text,
    token_type text,
    scope text,
    expires_at timestamp with time zone,
    metadata jsonb DEFAULT '{}'::jsonb,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    code text,
    connection_name text NOT NULL,
    provider_url text NOT NULL,
    client_secret text,
    pat_token text,
    projects_cache jsonb,
    org_id bigint DEFAULT 1 NOT NULL
);


--
-- Name: integration_tokens_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.integration_tokens_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: integration_tokens_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.integration_tokens_id_seq OWNED BY public.integration_tokens.id;


--
-- Name: orgs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.orgs (
    id bigint NOT NULL,
    name character varying(255) NOT NULL,
    description text,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now(),
    settings jsonb DEFAULT '{}'::jsonb,
    is_active boolean DEFAULT true NOT NULL,
    created_by_user_id bigint,
    subscription_plan character varying(50) DEFAULT 'free'::character varying,
    max_users integer DEFAULT 10
);


--
-- Name: orgs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.orgs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: orgs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.orgs_id_seq OWNED BY public.orgs.id;


--
-- Name: prompt_application_context; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.prompt_application_context (
    id bigint NOT NULL,
    org_id bigint NOT NULL,
    ai_connector_id integer,
    integration_token_id bigint,
    group_identifier text,
    repository text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: prompt_application_context_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.prompt_application_context_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: prompt_application_context_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.prompt_application_context_id_seq OWNED BY public.prompt_application_context.id;


--
-- Name: prompt_chunks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.prompt_chunks (
    id bigint NOT NULL,
    org_id bigint NOT NULL,
    application_context_id bigint NOT NULL,
    prompt_key text NOT NULL,
    variable_name text NOT NULL,
    chunk_type text NOT NULL,
    title text,
    body text NOT NULL,
    sequence_index integer DEFAULT 1000 NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    allow_markdown boolean DEFAULT true NOT NULL,
    redact_on_log boolean DEFAULT false NOT NULL,
    created_by bigint,
    updated_by bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: prompt_chunks_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.prompt_chunks_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: prompt_chunks_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.prompt_chunks_id_seq OWNED BY public.prompt_chunks.id;


--
-- Name: recent_activity; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.recent_activity (
    id integer NOT NULL,
    activity_type character varying(50) NOT NULL,
    event_data jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    review_id bigint,
    org_id bigint DEFAULT 1 NOT NULL
);


--
-- Name: recent_activity_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.recent_activity_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: recent_activity_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.recent_activity_id_seq OWNED BY public.recent_activity.id;


--
-- Name: reviews; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reviews (
    id bigint NOT NULL,
    repository character varying(255) NOT NULL,
    branch character varying(255),
    commit_hash character varying(255),
    pr_mr_url text,
    connector_id bigint,
    status character varying(50) DEFAULT 'created'::character varying NOT NULL,
    trigger_type character varying(50) DEFAULT 'manual'::character varying NOT NULL,
    user_email character varying(255),
    provider character varying(100),
    created_at timestamp with time zone DEFAULT now(),
    started_at timestamp with time zone,
    completed_at timestamp with time zone,
    metadata jsonb DEFAULT '{}'::jsonb,
    org_id bigint DEFAULT 1 NOT NULL,
    CONSTRAINT reviews_status_check CHECK (((status)::text = ANY ((ARRAY['created'::character varying, 'in_progress'::character varying, 'completed'::character varying, 'failed'::character varying])::text[])))
);


--
-- Name: reviews_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.reviews_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: reviews_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.reviews_id_seq OWNED BY public.reviews.id;


--
-- Name: roles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.roles (
    id bigint NOT NULL,
    name character varying(50) NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);


--
-- Name: roles_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.roles_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: roles_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.roles_id_seq OWNED BY public.roles.id;


--
-- Name: schema_migrations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.schema_migrations (
    version character varying NOT NULL
);


--
-- Name: user_management_audit; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_management_audit (
    id bigint NOT NULL,
    org_id bigint NOT NULL,
    target_user_id bigint NOT NULL,
    performed_by_user_id bigint NOT NULL,
    action character varying(50) NOT NULL,
    details jsonb DEFAULT '{}'::jsonb,
    created_at timestamp without time zone DEFAULT now() NOT NULL
);


--
-- Name: user_management_audit_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.user_management_audit_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: user_management_audit_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.user_management_audit_id_seq OWNED BY public.user_management_audit.id;


--
-- Name: user_role_history; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_role_history (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    org_id bigint NOT NULL,
    old_role_id bigint,
    new_role_id bigint NOT NULL,
    changed_by_user_id bigint NOT NULL,
    reason text,
    created_at timestamp without time zone DEFAULT now() NOT NULL
);


--
-- Name: user_role_history_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.user_role_history_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: user_role_history_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.user_role_history_id_seq OWNED BY public.user_role_history.id;


--
-- Name: user_roles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_roles (
    user_id bigint NOT NULL,
    role_id bigint NOT NULL,
    org_id bigint NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);


--
-- Name: users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.users (
    id bigint NOT NULL,
    email character varying(255) NOT NULL,
    password_hash character varying(255) NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now(),
    first_name character varying(100),
    last_name character varying(100),
    is_active boolean DEFAULT true NOT NULL,
    last_login_at timestamp without time zone,
    created_by_user_id bigint,
    deactivated_at timestamp without time zone,
    deactivated_by_user_id bigint,
    password_reset_required boolean DEFAULT false NOT NULL
);


--
-- Name: users_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.users_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: users_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.users_id_seq OWNED BY public.users.id;


--
-- Name: webhook_registry; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.webhook_registry (
    id integer NOT NULL,
    provider text NOT NULL,
    provider_project_id text NOT NULL,
    project_name text NOT NULL,
    project_full_name text NOT NULL,
    webhook_id text NOT NULL,
    webhook_url text NOT NULL,
    webhook_secret text,
    webhook_name text,
    events text,
    status text,
    last_verified_at timestamp without time zone,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    integration_token_id bigint,
    org_id bigint DEFAULT 1 NOT NULL
);


--
-- Name: webhook_registry_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.webhook_registry_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: webhook_registry_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.webhook_registry_id_seq OWNED BY public.webhook_registry.id;


--
-- Name: ai_comments id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_comments ALTER COLUMN id SET DEFAULT nextval('public.ai_comments_id_seq'::regclass);


--
-- Name: ai_connectors id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_connectors ALTER COLUMN id SET DEFAULT nextval('public.ai_connectors_id_seq'::regclass);


--
-- Name: auth_tokens id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.auth_tokens ALTER COLUMN id SET DEFAULT nextval('public.auth_tokens_id_seq'::regclass);


--
-- Name: instance_details id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.instance_details ALTER COLUMN id SET DEFAULT nextval('public.instance_details_id_seq'::regclass);


--
-- Name: integration_tokens id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.integration_tokens ALTER COLUMN id SET DEFAULT nextval('public.integration_tokens_id_seq'::regclass);


--
-- Name: orgs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.orgs ALTER COLUMN id SET DEFAULT nextval('public.orgs_id_seq'::regclass);


--
-- Name: prompt_application_context id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prompt_application_context ALTER COLUMN id SET DEFAULT nextval('public.prompt_application_context_id_seq'::regclass);


--
-- Name: prompt_chunks id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prompt_chunks ALTER COLUMN id SET DEFAULT nextval('public.prompt_chunks_id_seq'::regclass);


--
-- Name: recent_activity id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.recent_activity ALTER COLUMN id SET DEFAULT nextval('public.recent_activity_id_seq'::regclass);


--
-- Name: reviews id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reviews ALTER COLUMN id SET DEFAULT nextval('public.reviews_id_seq'::regclass);


--
-- Name: roles id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.roles ALTER COLUMN id SET DEFAULT nextval('public.roles_id_seq'::regclass);


--
-- Name: user_management_audit id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_management_audit ALTER COLUMN id SET DEFAULT nextval('public.user_management_audit_id_seq'::regclass);


--
-- Name: user_role_history id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_role_history ALTER COLUMN id SET DEFAULT nextval('public.user_role_history_id_seq'::regclass);


--
-- Name: users id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users ALTER COLUMN id SET DEFAULT nextval('public.users_id_seq'::regclass);


--
-- Name: webhook_registry id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.webhook_registry ALTER COLUMN id SET DEFAULT nextval('public.webhook_registry_id_seq'::regclass);


--
-- Name: ai_comments ai_comments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_comments
    ADD CONSTRAINT ai_comments_pkey PRIMARY KEY (id);


--
-- Name: ai_connectors ai_connectors_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_connectors
    ADD CONSTRAINT ai_connectors_pkey PRIMARY KEY (id);


--
-- Name: auth_tokens auth_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.auth_tokens
    ADD CONSTRAINT auth_tokens_pkey PRIMARY KEY (id);


--
-- Name: dashboard_cache dashboard_cache_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dashboard_cache
    ADD CONSTRAINT dashboard_cache_pkey PRIMARY KEY (id);


--
-- Name: instance_details instance_details_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.instance_details
    ADD CONSTRAINT instance_details_pkey PRIMARY KEY (id);


--
-- Name: integration_tokens integration_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.integration_tokens
    ADD CONSTRAINT integration_tokens_pkey PRIMARY KEY (id);


--
-- Name: orgs orgs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.orgs
    ADD CONSTRAINT orgs_pkey PRIMARY KEY (id);


--
-- Name: prompt_application_context prompt_application_context_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prompt_application_context
    ADD CONSTRAINT prompt_application_context_pkey PRIMARY KEY (id);


--
-- Name: prompt_chunks prompt_chunks_application_context_id_prompt_key_variable_na_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prompt_chunks
    ADD CONSTRAINT prompt_chunks_application_context_id_prompt_key_variable_na_key UNIQUE (application_context_id, prompt_key, variable_name, sequence_index);


--
-- Name: prompt_chunks prompt_chunks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prompt_chunks
    ADD CONSTRAINT prompt_chunks_pkey PRIMARY KEY (id);


--
-- Name: recent_activity recent_activity_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.recent_activity
    ADD CONSTRAINT recent_activity_pkey PRIMARY KEY (id);


--
-- Name: reviews reviews_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reviews
    ADD CONSTRAINT reviews_pkey PRIMARY KEY (id);


--
-- Name: roles roles_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT roles_name_key UNIQUE (name);


--
-- Name: roles roles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT roles_pkey PRIMARY KEY (id);


--
-- Name: schema_migrations schema_migrations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.schema_migrations
    ADD CONSTRAINT schema_migrations_pkey PRIMARY KEY (version);


--
-- Name: user_management_audit user_management_audit_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_management_audit
    ADD CONSTRAINT user_management_audit_pkey PRIMARY KEY (id);


--
-- Name: user_role_history user_role_history_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_role_history
    ADD CONSTRAINT user_role_history_pkey PRIMARY KEY (id);


--
-- Name: user_roles user_roles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_roles
    ADD CONSTRAINT user_roles_pkey PRIMARY KEY (user_id, role_id, org_id);


--
-- Name: users users_email_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_email_key UNIQUE (email);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: webhook_registry webhook_registry_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.webhook_registry
    ADD CONSTRAINT webhook_registry_pkey PRIMARY KEY (id);


--
-- Name: idx_ai_comments_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_comments_created_at ON public.ai_comments USING btree (created_at DESC);


--
-- Name: idx_ai_comments_file_path; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_comments_file_path ON public.ai_comments USING btree (file_path) WHERE (file_path IS NOT NULL);


--
-- Name: idx_ai_comments_org_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_comments_org_created ON public.ai_comments USING btree (org_id, created_at DESC);


--
-- Name: idx_ai_comments_org_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_comments_org_id ON public.ai_comments USING btree (org_id);


--
-- Name: idx_ai_comments_org_review; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_comments_org_review ON public.ai_comments USING btree (org_id, review_id);


--
-- Name: idx_ai_comments_review_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_comments_review_id ON public.ai_comments USING btree (review_id);


--
-- Name: idx_ai_comments_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_comments_type ON public.ai_comments USING btree (comment_type);


--
-- Name: idx_ai_connectors_org_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_connectors_org_id ON public.ai_connectors USING btree (org_id);


--
-- Name: idx_ai_connectors_org_provider; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_connectors_org_provider ON public.ai_connectors USING btree (org_id, provider_name);


--
-- Name: idx_ai_connectors_provider_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_connectors_provider_name ON public.ai_connectors USING btree (provider_name);


--
-- Name: idx_audit_org_action; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_audit_org_action ON public.user_management_audit USING btree (org_id, action, created_at DESC);


--
-- Name: idx_audit_performed_by; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_audit_performed_by ON public.user_management_audit USING btree (performed_by_user_id, created_at DESC);


--
-- Name: idx_audit_target_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_audit_target_time ON public.user_management_audit USING btree (target_user_id, created_at DESC);


--
-- Name: idx_auth_tokens_active_sessions; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_auth_tokens_active_sessions ON public.auth_tokens USING btree (user_id, last_used_at) WHERE (((token_type)::text = 'session'::text) AND (is_active = true));


--
-- Name: idx_auth_tokens_cleanup; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_auth_tokens_cleanup ON public.auth_tokens USING btree (token_type, expires_at, is_active);


--
-- Name: idx_auth_tokens_expires; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_auth_tokens_expires ON public.auth_tokens USING btree (expires_at) WHERE (is_active = true);


--
-- Name: idx_auth_tokens_hash; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_auth_tokens_hash ON public.auth_tokens USING btree (token_hash) WHERE (is_active = true);


--
-- Name: idx_auth_tokens_last_used; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_auth_tokens_last_used ON public.auth_tokens USING btree (last_used_at) WHERE (is_active = true);


--
-- Name: idx_auth_tokens_refresh; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_auth_tokens_refresh ON public.auth_tokens USING btree (token_hash, token_type) WHERE (((token_type)::text = 'refresh'::text) AND (is_active = true));


--
-- Name: idx_auth_tokens_type_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_auth_tokens_type_user ON public.auth_tokens USING btree (token_type, user_id) WHERE (is_active = true);


--
-- Name: idx_auth_tokens_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_auth_tokens_user_id ON public.auth_tokens USING btree (user_id);


--
-- Name: idx_chunks_appctx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_chunks_appctx ON public.prompt_chunks USING btree (application_context_id);


--
-- Name: idx_chunks_prompt_var; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_chunks_prompt_var ON public.prompt_chunks USING btree (prompt_key, variable_name);


--
-- Name: idx_dashboard_cache_org_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_dashboard_cache_org_id ON public.dashboard_cache USING btree (org_id);


--
-- Name: idx_dashboard_cache_org_updated; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_dashboard_cache_org_updated ON public.dashboard_cache USING btree (org_id, updated_at DESC);


--
-- Name: idx_integration_tokens_org_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_integration_tokens_org_created ON public.integration_tokens USING btree (org_id, created_at);


--
-- Name: idx_integration_tokens_org_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_integration_tokens_org_id ON public.integration_tokens USING btree (org_id);


--
-- Name: idx_integration_tokens_org_provider; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_integration_tokens_org_provider ON public.integration_tokens USING btree (org_id, provider);


--
-- Name: idx_orgs_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_orgs_active ON public.orgs USING btree (is_active, created_at);


--
-- Name: idx_orgs_plan; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_orgs_plan ON public.orgs USING btree (subscription_plan, is_active);


--
-- Name: idx_orgs_settings; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_orgs_settings ON public.orgs USING gin (settings) WHERE (settings IS NOT NULL);


--
-- Name: idx_pac_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pac_org ON public.prompt_application_context USING btree (org_id);


--
-- Name: idx_pac_targeting; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pac_targeting ON public.prompt_application_context USING btree (org_id, ai_connector_id, integration_token_id, group_identifier, repository);


--
-- Name: idx_recent_activity_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_recent_activity_created_at ON public.recent_activity USING btree (created_at DESC);


--
-- Name: idx_recent_activity_dashboard; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_recent_activity_dashboard ON public.recent_activity USING btree (created_at DESC, activity_type);


--
-- Name: idx_recent_activity_org_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_recent_activity_org_created ON public.recent_activity USING btree (org_id, created_at DESC);


--
-- Name: idx_recent_activity_org_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_recent_activity_org_id ON public.recent_activity USING btree (org_id);


--
-- Name: idx_recent_activity_org_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_recent_activity_org_type ON public.recent_activity USING btree (org_id, activity_type);


--
-- Name: idx_recent_activity_review_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_recent_activity_review_id ON public.recent_activity USING btree (review_id);


--
-- Name: idx_recent_activity_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_recent_activity_type ON public.recent_activity USING btree (activity_type);


--
-- Name: idx_reviews_connector_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reviews_connector_id ON public.reviews USING btree (connector_id);


--
-- Name: idx_reviews_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reviews_created_at ON public.reviews USING btree (created_at DESC);


--
-- Name: idx_reviews_org_connector; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reviews_org_connector ON public.reviews USING btree (org_id, connector_id);


--
-- Name: idx_reviews_org_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reviews_org_created ON public.reviews USING btree (org_id, created_at DESC);


--
-- Name: idx_reviews_org_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reviews_org_id ON public.reviews USING btree (org_id);


--
-- Name: idx_reviews_org_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reviews_org_status ON public.reviews USING btree (org_id, status);


--
-- Name: idx_reviews_provider; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reviews_provider ON public.reviews USING btree (provider);


--
-- Name: idx_reviews_repository; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reviews_repository ON public.reviews USING btree (repository);


--
-- Name: idx_reviews_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_reviews_status ON public.reviews USING btree (status);


--
-- Name: idx_user_role_history_changed_by; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_role_history_changed_by ON public.user_role_history USING btree (changed_by_user_id, created_at);


--
-- Name: idx_user_role_history_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_role_history_org ON public.user_role_history USING btree (org_id, created_at);


--
-- Name: idx_user_role_history_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_role_history_user ON public.user_role_history USING btree (user_id, created_at);


--
-- Name: idx_user_roles_org_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_roles_org_id ON public.user_roles USING btree (org_id);


--
-- Name: idx_user_roles_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_roles_user_id ON public.user_roles USING btree (user_id);


--
-- Name: idx_user_roles_user_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_roles_user_org ON public.user_roles USING btree (user_id, org_id);


--
-- Name: idx_users_created_by; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_created_by ON public.users USING btree (created_by_user_id, created_at DESC);


--
-- Name: idx_users_email; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_email ON public.users USING btree (email);


--
-- Name: idx_users_last_login; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_last_login ON public.users USING btree (last_login_at DESC) WHERE (is_active = true);


--
-- Name: idx_users_org_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_org_active ON public.users USING btree (id) WHERE (is_active = true);


--
-- Name: idx_users_password_reset; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_password_reset ON public.users USING btree (id) WHERE (password_reset_required = true);


--
-- Name: idx_webhook_registry_integration_token_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_webhook_registry_integration_token_id ON public.webhook_registry USING btree (integration_token_id);


--
-- Name: idx_webhook_registry_org_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_webhook_registry_org_id ON public.webhook_registry USING btree (org_id);


--
-- Name: idx_webhook_registry_org_provider; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_webhook_registry_org_provider ON public.webhook_registry USING btree (org_id, provider);


--
-- Name: idx_webhook_registry_org_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_webhook_registry_org_status ON public.webhook_registry USING btree (org_id, status);


--
-- Name: idx_webhook_registry_provider_project; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_webhook_registry_provider_project ON public.webhook_registry USING btree (provider, provider_project_id);


--
-- Name: ai_comments ai_comments_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_comments
    ADD CONSTRAINT ai_comments_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id);


--
-- Name: ai_comments ai_comments_review_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_comments
    ADD CONSTRAINT ai_comments_review_id_fkey FOREIGN KEY (review_id) REFERENCES public.reviews(id) ON DELETE CASCADE;


--
-- Name: ai_connectors ai_connectors_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_connectors
    ADD CONSTRAINT ai_connectors_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id);


--
-- Name: auth_tokens auth_tokens_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.auth_tokens
    ADD CONSTRAINT auth_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: dashboard_cache dashboard_cache_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dashboard_cache
    ADD CONSTRAINT dashboard_cache_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id);


--
-- Name: webhook_registry fk_webhook_registry_integration_token; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.webhook_registry
    ADD CONSTRAINT fk_webhook_registry_integration_token FOREIGN KEY (integration_token_id) REFERENCES public.integration_tokens(id) ON DELETE CASCADE;


--
-- Name: integration_tokens integration_tokens_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.integration_tokens
    ADD CONSTRAINT integration_tokens_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id);


--
-- Name: orgs orgs_created_by_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.orgs
    ADD CONSTRAINT orgs_created_by_user_id_fkey FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: prompt_application_context prompt_application_context_ai_connector_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prompt_application_context
    ADD CONSTRAINT prompt_application_context_ai_connector_id_fkey FOREIGN KEY (ai_connector_id) REFERENCES public.ai_connectors(id);


--
-- Name: prompt_application_context prompt_application_context_integration_token_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prompt_application_context
    ADD CONSTRAINT prompt_application_context_integration_token_id_fkey FOREIGN KEY (integration_token_id) REFERENCES public.integration_tokens(id);


--
-- Name: prompt_application_context prompt_application_context_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prompt_application_context
    ADD CONSTRAINT prompt_application_context_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id);


--
-- Name: prompt_chunks prompt_chunks_application_context_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prompt_chunks
    ADD CONSTRAINT prompt_chunks_application_context_id_fkey FOREIGN KEY (application_context_id) REFERENCES public.prompt_application_context(id) ON DELETE CASCADE;


--
-- Name: prompt_chunks prompt_chunks_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prompt_chunks
    ADD CONSTRAINT prompt_chunks_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id);


--
-- Name: recent_activity recent_activity_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.recent_activity
    ADD CONSTRAINT recent_activity_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id);


--
-- Name: recent_activity recent_activity_review_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.recent_activity
    ADD CONSTRAINT recent_activity_review_id_fkey FOREIGN KEY (review_id) REFERENCES public.reviews(id) ON DELETE SET NULL;


--
-- Name: reviews reviews_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reviews
    ADD CONSTRAINT reviews_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id);


--
-- Name: user_management_audit user_management_audit_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_management_audit
    ADD CONSTRAINT user_management_audit_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;


--
-- Name: user_management_audit user_management_audit_performed_by_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_management_audit
    ADD CONSTRAINT user_management_audit_performed_by_user_id_fkey FOREIGN KEY (performed_by_user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: user_management_audit user_management_audit_target_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_management_audit
    ADD CONSTRAINT user_management_audit_target_user_id_fkey FOREIGN KEY (target_user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: user_role_history user_role_history_changed_by_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_role_history
    ADD CONSTRAINT user_role_history_changed_by_user_id_fkey FOREIGN KEY (changed_by_user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: user_role_history user_role_history_new_role_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_role_history
    ADD CONSTRAINT user_role_history_new_role_id_fkey FOREIGN KEY (new_role_id) REFERENCES public.roles(id) ON DELETE CASCADE;


--
-- Name: user_role_history user_role_history_old_role_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_role_history
    ADD CONSTRAINT user_role_history_old_role_id_fkey FOREIGN KEY (old_role_id) REFERENCES public.roles(id) ON DELETE SET NULL;


--
-- Name: user_role_history user_role_history_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_role_history
    ADD CONSTRAINT user_role_history_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;


--
-- Name: user_role_history user_role_history_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_role_history
    ADD CONSTRAINT user_role_history_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: user_roles user_roles_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_roles
    ADD CONSTRAINT user_roles_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id);


--
-- Name: user_roles user_roles_role_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_roles
    ADD CONSTRAINT user_roles_role_id_fkey FOREIGN KEY (role_id) REFERENCES public.roles(id);


--
-- Name: user_roles user_roles_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_roles
    ADD CONSTRAINT user_roles_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: users users_created_by_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_created_by_user_id_fkey FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: users users_deactivated_by_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_deactivated_by_user_id_fkey FOREIGN KEY (deactivated_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: webhook_registry webhook_registry_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.webhook_registry
    ADD CONSTRAINT webhook_registry_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id);


--
-- PostgreSQL database dump complete
--


--
-- Dbmate schema migrations
--

INSERT INTO public.schema_migrations (version) VALUES
    ('20250719000001'),
    ('20250719000002'),
    ('20250719000003'),
    ('20250719000004'),
    ('20250720000001'),
    ('20250720000002'),
    ('20250720135317'),
    ('20250720182946'),
    ('20250721035816'),
    ('20250721141011'),
    ('20250722035359'),
    ('20250722040308'),
    ('20250722064012'),
    ('20250723093453'),
    ('20250728092945'),
    ('20250728093051'),
    ('20250731131105'),
    ('20250801150601'),
    ('20250805104629'),
    ('20250811091248'),
    ('20250811145541'),
    ('20250811145851'),
    ('20250815000001'),
    ('20250827180852'),
    ('20250827180901'),
    ('20250828094624'),
    ('20250828105719'),
    ('20250828112835'),
    ('20250828112941'),
    ('20250828113024'),
    ('20250905120000');
