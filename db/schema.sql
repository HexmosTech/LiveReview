\restrict dbmate

-- Dumped from database version 15.17 (Debian 15.17-1.pgdg13+1)
-- Dumped by pg_dump version 16.13 (Ubuntu 16.13-0ubuntu0.24.04.1)

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

--
-- Name: learning_scope; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.learning_scope AS ENUM (
    'org',
    'repo'
);


--
-- Name: learning_status; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.learning_status AS ENUM (
    'active',
    'archived'
);


--
-- Name: river_job_state; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.river_job_state AS ENUM (
    'available',
    'cancelled',
    'completed',
    'discarded',
    'pending',
    'retryable',
    'running',
    'scheduled'
);


--
-- Name: license_seat_assignments_set_updated_at(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.license_seat_assignments_set_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$;


--
-- Name: license_state_set_updated_at(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.license_state_set_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$;


--
-- Name: org_billing_state_set_updated_at(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.org_billing_state_set_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$;


--
-- Name: plan_catalog_set_updated_at(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.plan_catalog_set_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$;


--
-- Name: quota_batch_settlements_set_updated_at(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.quota_batch_settlements_set_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$;


--
-- Name: quota_operation_aggregates_set_updated_at(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.quota_operation_aggregates_set_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$;


--
-- Name: quota_policy_catalog_set_updated_at(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.quota_policy_catalog_set_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$;


--
-- Name: river_job_state_in_bitmask(bit, public.river_job_state); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.river_job_state_in_bitmask(bitmask bit, state public.river_job_state) RETURNS boolean
    LANGUAGE sql IMMUTABLE
    AS $$
    SELECT CASE state
        WHEN 'available' THEN get_bit(bitmask, 7)
        WHEN 'cancelled' THEN get_bit(bitmask, 6)
        WHEN 'completed' THEN get_bit(bitmask, 5)
        WHEN 'discarded' THEN get_bit(bitmask, 4)
        WHEN 'pending'   THEN get_bit(bitmask, 3)
        WHEN 'retryable' THEN get_bit(bitmask, 2)
        WHEN 'running'   THEN get_bit(bitmask, 1)
        WHEN 'scheduled' THEN get_bit(bitmask, 0)
        ELSE 0
    END = 1;
$$;


--
-- Name: trial_eligibility_set_updated_at(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.trial_eligibility_set_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
	NEW.updated_at = NOW();
	RETURN NEW;
END;
$$;


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
-- Name: api_keys; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.api_keys (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    org_id bigint NOT NULL,
    key_hash character varying(128) NOT NULL,
    key_prefix character varying(16) NOT NULL,
    label character varying(255) NOT NULL,
    scopes jsonb DEFAULT '[]'::jsonb,
    last_used_at timestamp without time zone,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    expires_at timestamp without time zone,
    revoked_at timestamp without time zone,
    CONSTRAINT valid_expiry CHECK (((expires_at IS NULL) OR (expires_at > created_at)))
);


--
-- Name: TABLE api_keys; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.api_keys IS 'Personal API keys for programmatic access';


--
-- Name: COLUMN api_keys.key_hash; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.api_keys.key_hash IS 'SHA-256 hash of the API key';


--
-- Name: COLUMN api_keys.key_prefix; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.api_keys.key_prefix IS 'First 8 chars of the key for display purposes';


--
-- Name: COLUMN api_keys.label; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.api_keys.label IS 'User-provided label for the key';


--
-- Name: COLUMN api_keys.scopes; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.api_keys.scopes IS 'JSON array of scope strings (e.g., ["read", "write"])';


--
-- Name: COLUMN api_keys.last_used_at; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.api_keys.last_used_at IS 'Timestamp of last successful authentication';


--
-- Name: api_keys_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.api_keys_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: api_keys_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.api_keys_id_seq OWNED BY public.api_keys.id;


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
-- Name: billing_notification_outbox; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.billing_notification_outbox (
    id bigint NOT NULL,
    org_id bigint NOT NULL,
    event_type character varying(80) NOT NULL,
    channel character varying(24) NOT NULL,
    dedupe_key character varying(255) NOT NULL,
    payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    recipient_user_id bigint,
    recipient_email character varying(320),
    status character varying(32) DEFAULT 'pending'::character varying NOT NULL,
    retry_count integer DEFAULT 0 NOT NULL,
    last_error text,
    send_after timestamp with time zone DEFAULT now() NOT NULL,
    sent_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT chk_billing_notification_outbox_channel CHECK (((channel)::text = ANY ((ARRAY['in_app'::character varying, 'email'::character varying])::text[]))),
    CONSTRAINT chk_billing_notification_outbox_status CHECK (((status)::text = ANY ((ARRAY['pending'::character varying, 'processing'::character varying, 'sent'::character varying, 'failed'::character varying, 'cancelled'::character varying])::text[])))
);


--
-- Name: billing_notification_outbox_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.billing_notification_outbox_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: billing_notification_outbox_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.billing_notification_outbox_id_seq OWNED BY public.billing_notification_outbox.id;


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
-- Name: learning_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.learning_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    learning_id uuid NOT NULL,
    org_id bigint NOT NULL,
    action text NOT NULL,
    provider text NOT NULL,
    thread_id text,
    comment_id text,
    repository text,
    commit_sha text,
    file_path text,
    line_start integer,
    line_end integer,
    actor_id bigint,
    reason_snippet text,
    classifier text,
    context jsonb,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT learning_events_action_check CHECK ((action = ANY (ARRAY['add'::text, 'update'::text, 'delete'::text, 'restore'::text])))
);


--
-- Name: learnings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.learnings (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    short_id text NOT NULL,
    org_id bigint NOT NULL,
    scope_kind public.learning_scope NOT NULL,
    repo_id text,
    title text NOT NULL,
    body text NOT NULL,
    tags text[] DEFAULT '{}'::text[] NOT NULL,
    status public.learning_status DEFAULT 'active'::public.learning_status NOT NULL,
    confidence integer DEFAULT 1 NOT NULL,
    simhash bigint NOT NULL,
    embedding bytea,
    tsv tsvector GENERATED ALWAYS AS (to_tsvector('simple'::regconfig, ((COALESCE(title, ''::text) || ' '::text) || COALESCE(body, ''::text)))) STORED,
    source_urls text[] DEFAULT '{}'::text[] NOT NULL,
    source_context jsonb,
    created_by bigint,
    updated_by bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: license_log; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.license_log (
    id bigint NOT NULL,
    subscription_id bigint,
    user_id bigint,
    org_id bigint,
    event_type character varying(100) NOT NULL,
    actor_id bigint,
    razorpay_event_id character varying(255),
    metadata jsonb,
    processed boolean DEFAULT true,
    processed_at timestamp with time zone,
    error_message text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    description text
);


--
-- Name: license_log_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.license_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: license_log_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.license_log_id_seq OWNED BY public.license_log.id;


--
-- Name: license_seat_assignments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.license_seat_assignments (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    assigned_by_user_id bigint,
    assigned_at timestamp with time zone DEFAULT now() NOT NULL,
    revoked_at timestamp with time zone,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: license_seat_assignments_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.license_seat_assignments_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: license_seat_assignments_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.license_seat_assignments_id_seq OWNED BY public.license_seat_assignments.id;


--
-- Name: license_state; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.license_state (
    id smallint DEFAULT 1 NOT NULL,
    token text,
    kid character varying(32),
    subject character varying(255),
    app_name character varying(128),
    seat_count integer,
    unlimited boolean DEFAULT false,
    issued_at timestamp with time zone,
    expires_at timestamp with time zone,
    last_validated_at timestamp with time zone,
    last_validation_error_code character varying(64),
    validation_failures integer DEFAULT 0,
    status character varying(32) DEFAULT 'missing'::character varying NOT NULL,
    grace_started_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);


--
-- Name: loc_lifecycle_log; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.loc_lifecycle_log (
    id bigint NOT NULL,
    org_id bigint NOT NULL,
    event_type character varying(80) NOT NULL,
    threshold_percent integer,
    usage_ledger_id bigint,
    plan_code character varying(64),
    event_key character varying(255) NOT NULL,
    payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    notified_email boolean DEFAULT false NOT NULL,
    notified_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT chk_loc_lifecycle_threshold_range CHECK (((threshold_percent IS NULL) OR ((threshold_percent >= 0) AND (threshold_percent <= 100))))
);


--
-- Name: loc_lifecycle_log_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.loc_lifecycle_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: loc_lifecycle_log_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.loc_lifecycle_log_id_seq OWNED BY public.loc_lifecycle_log.id;


--
-- Name: loc_usage_ledger; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.loc_usage_ledger (
    id bigint NOT NULL,
    org_id bigint NOT NULL,
    review_id bigint,
    user_id bigint,
    operation_type character varying(64) NOT NULL,
    trigger_source character varying(64) NOT NULL,
    operation_id character varying(128) NOT NULL,
    idempotency_key character varying(255) NOT NULL,
    billable_loc bigint NOT NULL,
    accounted_at timestamp with time zone DEFAULT now() NOT NULL,
    billing_period_start timestamp with time zone NOT NULL,
    billing_period_end timestamp with time zone NOT NULL,
    status character varying(32) DEFAULT 'accounted'::character varying NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    provider character varying(64),
    model character varying(128),
    pricing_version character varying(64),
    input_tokens bigint,
    output_tokens bigint,
    llm_cost_usd double precision,
    actor_kind character varying(16),
    actor_email_snapshot character varying(320),
    CONSTRAINT chk_loc_usage_ledger_actor_kind CHECK (((actor_kind IS NULL) OR ((actor_kind)::text = ANY ((ARRAY['member'::character varying, 'system'::character varying, 'unknown'::character varying])::text[])))),
    CONSTRAINT chk_loc_usage_ledger_billable_positive CHECK ((billable_loc > 0)),
    CONSTRAINT chk_loc_usage_ledger_cost_non_negative CHECK (((llm_cost_usd IS NULL) OR (llm_cost_usd >= (0)::double precision))),
    CONSTRAINT chk_loc_usage_ledger_input_tokens_non_negative CHECK (((input_tokens IS NULL) OR (input_tokens >= 0))),
    CONSTRAINT chk_loc_usage_ledger_output_tokens_non_negative CHECK (((output_tokens IS NULL) OR (output_tokens >= 0))),
    CONSTRAINT chk_loc_usage_ledger_period_valid CHECK ((billing_period_end > billing_period_start)),
    CONSTRAINT chk_loc_usage_ledger_status_valid CHECK (((status)::text = ANY ((ARRAY['accounted'::character varying, 'ignored'::character varying])::text[])))
);


--
-- Name: loc_usage_ledger_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.loc_usage_ledger_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: loc_usage_ledger_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.loc_usage_ledger_id_seq OWNED BY public.loc_usage_ledger.id;


--
-- Name: org_billing_state; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.org_billing_state (
    id bigint NOT NULL,
    org_id bigint NOT NULL,
    current_plan_code character varying(64) NOT NULL,
    billing_period_start timestamp with time zone NOT NULL,
    billing_period_end timestamp with time zone NOT NULL,
    loc_used_month bigint DEFAULT 0 NOT NULL,
    loc_blocked boolean DEFAULT false NOT NULL,
    trial_started_at timestamp with time zone,
    trial_ends_at timestamp with time zone,
    trial_readonly boolean DEFAULT false NOT NULL,
    scheduled_plan_code character varying(64),
    scheduled_plan_effective_at timestamp with time zone,
    last_reset_at timestamp with time zone DEFAULT now() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    upgrade_loc_grant_current_cycle bigint DEFAULT 0 NOT NULL,
    upgrade_loc_grant_expires_at timestamp with time zone,
    CONSTRAINT chk_org_billing_loc_used_non_negative CHECK ((loc_used_month >= 0)),
    CONSTRAINT chk_org_billing_period_valid CHECK ((billing_period_end > billing_period_start)),
    CONSTRAINT chk_org_billing_schedule_pair CHECK ((((scheduled_plan_code IS NULL) AND (scheduled_plan_effective_at IS NULL)) OR ((scheduled_plan_code IS NOT NULL) AND (scheduled_plan_effective_at IS NOT NULL)))),
    CONSTRAINT chk_org_billing_trial_window_valid CHECK (((trial_ends_at IS NULL) OR (trial_started_at IS NULL) OR (trial_ends_at > trial_started_at))),
    CONSTRAINT chk_org_billing_upgrade_loc_grant_non_negative CHECK ((upgrade_loc_grant_current_cycle >= 0))
);


--
-- Name: org_billing_state_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.org_billing_state_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: org_billing_state_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.org_billing_state_id_seq OWNED BY public.org_billing_state.id;


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
-- Name: plan_catalog; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.plan_catalog (
    id bigint NOT NULL,
    plan_code character varying(64) NOT NULL,
    display_name character varying(120) NOT NULL,
    active boolean DEFAULT true NOT NULL,
    rank integer NOT NULL,
    monthly_price_usd integer NOT NULL,
    monthly_loc_limit bigint NOT NULL,
    feature_flags jsonb DEFAULT '[]'::jsonb NOT NULL,
    trial_enabled boolean DEFAULT false NOT NULL,
    trial_days integer DEFAULT 0 NOT NULL,
    envelope_show_price boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT chk_plan_catalog_loc_non_negative CHECK ((monthly_loc_limit >= 0)),
    CONSTRAINT chk_plan_catalog_price_non_negative CHECK ((monthly_price_usd >= 0)),
    CONSTRAINT chk_plan_catalog_rank_non_negative CHECK ((rank >= 0)),
    CONSTRAINT chk_plan_catalog_trial_config CHECK ((((trial_enabled = true) AND (trial_days > 0)) OR (trial_enabled = false))),
    CONSTRAINT chk_plan_catalog_trial_days_non_negative CHECK ((trial_days >= 0))
);


--
-- Name: plan_catalog_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.plan_catalog_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: plan_catalog_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.plan_catalog_id_seq OWNED BY public.plan_catalog.id;


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
-- Name: quota_batch_settlements; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.quota_batch_settlements (
    id bigint NOT NULL,
    org_id bigint NOT NULL,
    review_id bigint,
    operation_type character varying(64) NOT NULL,
    trigger_source character varying(64) NOT NULL,
    operation_id character varying(128) NOT NULL,
    idempotency_key character varying(255) NOT NULL,
    batch_index integer NOT NULL,
    plan_code character varying(64) NOT NULL,
    policy_provider_key character varying(64) NOT NULL,
    pricing_version character varying(64) NOT NULL,
    raw_loc_batch bigint NOT NULL,
    effective_loc_batch bigint NOT NULL,
    extra_effective_loc_batch bigint NOT NULL,
    diff_input_tokens_batch bigint NOT NULL,
    context_chars_batch bigint NOT NULL,
    context_tokens_batch bigint NOT NULL,
    allowed_context_tokens_batch bigint NOT NULL,
    extra_context_tokens_batch bigint NOT NULL,
    provider_total_input_tokens_batch bigint NOT NULL,
    output_tokens_batch bigint NOT NULL,
    input_cost_usd_batch double precision NOT NULL,
    output_cost_usd_batch double precision NOT NULL,
    total_cost_usd_batch double precision NOT NULL,
    context_tokens_per_loc_allowance double precision NOT NULL,
    accounted_at timestamp with time zone DEFAULT now() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT chk_quota_batch_allowed_context_tokens_non_negative CHECK ((allowed_context_tokens_batch >= 0)),
    CONSTRAINT chk_quota_batch_context_allowance_non_negative CHECK ((context_tokens_per_loc_allowance >= (0)::double precision)),
    CONSTRAINT chk_quota_batch_context_chars_non_negative CHECK ((context_chars_batch >= 0)),
    CONSTRAINT chk_quota_batch_context_tokens_non_negative CHECK ((context_tokens_batch >= 0)),
    CONSTRAINT chk_quota_batch_diff_tokens_non_negative CHECK ((diff_input_tokens_batch >= 0)),
    CONSTRAINT chk_quota_batch_effective_loc_non_negative CHECK ((effective_loc_batch >= 0)),
    CONSTRAINT chk_quota_batch_extra_context_tokens_non_negative CHECK ((extra_context_tokens_batch >= 0)),
    CONSTRAINT chk_quota_batch_extra_loc_non_negative CHECK ((extra_effective_loc_batch >= 0)),
    CONSTRAINT chk_quota_batch_index_positive CHECK ((batch_index > 0)),
    CONSTRAINT chk_quota_batch_input_cost_non_negative CHECK ((input_cost_usd_batch >= (0)::double precision)),
    CONSTRAINT chk_quota_batch_output_cost_non_negative CHECK ((output_cost_usd_batch >= (0)::double precision)),
    CONSTRAINT chk_quota_batch_output_tokens_non_negative CHECK ((output_tokens_batch >= 0)),
    CONSTRAINT chk_quota_batch_provider_input_tokens_non_negative CHECK ((provider_total_input_tokens_batch >= 0)),
    CONSTRAINT chk_quota_batch_raw_loc_non_negative CHECK ((raw_loc_batch >= 0)),
    CONSTRAINT chk_quota_batch_total_cost_non_negative CHECK ((total_cost_usd_batch >= (0)::double precision))
);


--
-- Name: quota_batch_settlements_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.quota_batch_settlements_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: quota_batch_settlements_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.quota_batch_settlements_id_seq OWNED BY public.quota_batch_settlements.id;


--
-- Name: quota_operation_aggregates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.quota_operation_aggregates (
    id bigint NOT NULL,
    org_id bigint NOT NULL,
    review_id bigint,
    operation_type character varying(64) NOT NULL,
    trigger_source character varying(64) NOT NULL,
    operation_id character varying(128) NOT NULL,
    idempotency_key character varying(255) NOT NULL,
    plan_code character varying(64) NOT NULL,
    provider character varying(64),
    model character varying(128),
    pricing_version character varying(64) NOT NULL,
    batch_count integer NOT NULL,
    raw_loc_total bigint NOT NULL,
    effective_loc_total bigint NOT NULL,
    extra_effective_loc_total bigint NOT NULL,
    diff_input_tokens_total bigint NOT NULL,
    context_chars_total bigint NOT NULL,
    context_tokens_total bigint NOT NULL,
    allowed_context_tokens_total bigint NOT NULL,
    extra_context_tokens_total bigint NOT NULL,
    provider_total_input_tokens_total bigint NOT NULL,
    output_tokens_total bigint NOT NULL,
    input_cost_usd_total double precision NOT NULL,
    output_cost_usd_total double precision NOT NULL,
    total_cost_usd_total double precision NOT NULL,
    finalized_at timestamp with time zone DEFAULT now() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT chk_quota_operation_allowed_context_tokens_non_negative CHECK ((allowed_context_tokens_total >= 0)),
    CONSTRAINT chk_quota_operation_batch_count_positive CHECK ((batch_count > 0)),
    CONSTRAINT chk_quota_operation_context_chars_non_negative CHECK ((context_chars_total >= 0)),
    CONSTRAINT chk_quota_operation_context_tokens_non_negative CHECK ((context_tokens_total >= 0)),
    CONSTRAINT chk_quota_operation_diff_tokens_non_negative CHECK ((diff_input_tokens_total >= 0)),
    CONSTRAINT chk_quota_operation_effective_loc_non_negative CHECK ((effective_loc_total >= 0)),
    CONSTRAINT chk_quota_operation_extra_context_tokens_non_negative CHECK ((extra_context_tokens_total >= 0)),
    CONSTRAINT chk_quota_operation_extra_loc_non_negative CHECK ((extra_effective_loc_total >= 0)),
    CONSTRAINT chk_quota_operation_input_cost_non_negative CHECK ((input_cost_usd_total >= (0)::double precision)),
    CONSTRAINT chk_quota_operation_output_cost_non_negative CHECK ((output_cost_usd_total >= (0)::double precision)),
    CONSTRAINT chk_quota_operation_output_tokens_non_negative CHECK ((output_tokens_total >= 0)),
    CONSTRAINT chk_quota_operation_provider_input_tokens_non_negative CHECK ((provider_total_input_tokens_total >= 0)),
    CONSTRAINT chk_quota_operation_raw_loc_non_negative CHECK ((raw_loc_total >= 0)),
    CONSTRAINT chk_quota_operation_total_cost_non_negative CHECK ((total_cost_usd_total >= (0)::double precision))
);


--
-- Name: quota_operation_aggregates_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.quota_operation_aggregates_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: quota_operation_aggregates_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.quota_operation_aggregates_id_seq OWNED BY public.quota_operation_aggregates.id;


--
-- Name: quota_policy_catalog; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.quota_policy_catalog (
    id bigint NOT NULL,
    plan_code character varying(64) NOT NULL,
    provider_key character varying(64) NOT NULL,
    input_chars_per_loc integer NOT NULL,
    output_chars_per_loc integer NOT NULL,
    chars_per_token integer NOT NULL,
    loc_budget_ratio double precision NOT NULL,
    context_budget_ratio double precision NOT NULL,
    ops_reserved_ratio double precision NOT NULL,
    input_cost_per_million_tokens_usd double precision NOT NULL,
    output_cost_per_million_tokens_usd double precision NOT NULL,
    rounding_scale integer DEFAULT 6 NOT NULL,
    active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT chk_quota_policy_chars_per_token_positive CHECK ((chars_per_token > 0)),
    CONSTRAINT chk_quota_policy_context_budget_ratio CHECK (((context_budget_ratio >= (0)::double precision) AND (context_budget_ratio <= (1)::double precision))),
    CONSTRAINT chk_quota_policy_input_chars_positive CHECK ((input_chars_per_loc > 0)),
    CONSTRAINT chk_quota_policy_input_rate_non_negative CHECK ((input_cost_per_million_tokens_usd >= (0)::double precision)),
    CONSTRAINT chk_quota_policy_loc_budget_ratio CHECK (((loc_budget_ratio >= (0)::double precision) AND (loc_budget_ratio <= (1)::double precision))),
    CONSTRAINT chk_quota_policy_ops_reserved_ratio CHECK (((ops_reserved_ratio >= (0)::double precision) AND (ops_reserved_ratio <= (1)::double precision))),
    CONSTRAINT chk_quota_policy_output_chars_positive CHECK ((output_chars_per_loc > 0)),
    CONSTRAINT chk_quota_policy_output_rate_non_negative CHECK ((output_cost_per_million_tokens_usd >= (0)::double precision)),
    CONSTRAINT chk_quota_policy_ratio_sum CHECK ((abs((((loc_budget_ratio + context_budget_ratio) + ops_reserved_ratio) - (1.0)::double precision)) <= (0.000001)::double precision)),
    CONSTRAINT chk_quota_policy_rounding_scale CHECK (((rounding_scale >= 0) AND (rounding_scale <= 12)))
);


--
-- Name: quota_policy_catalog_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.quota_policy_catalog_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: quota_policy_catalog_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.quota_policy_catalog_id_seq OWNED BY public.quota_policy_catalog.id;


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
-- Name: review_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.review_events (
    id bigint NOT NULL,
    review_id bigint NOT NULL,
    org_id bigint NOT NULL,
    ts timestamp with time zone DEFAULT now() NOT NULL,
    event_type text NOT NULL,
    level text,
    batch_id text,
    data jsonb NOT NULL
);


--
-- Name: review_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.review_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: review_events_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.review_events_id_seq OWNED BY public.review_events.id;


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
    mr_title text,
    author_name text,
    author_username text,
    friendly_name text,
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
-- Name: river_client; Type: TABLE; Schema: public; Owner: -
--

CREATE UNLOGGED TABLE public.river_client (
    id text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    paused_at timestamp with time zone,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT name_length CHECK (((char_length(id) > 0) AND (char_length(id) < 128)))
);


--
-- Name: river_client_queue; Type: TABLE; Schema: public; Owner: -
--

CREATE UNLOGGED TABLE public.river_client_queue (
    river_client_id text NOT NULL,
    name text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    max_workers bigint DEFAULT 0 NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    num_jobs_completed bigint DEFAULT 0 NOT NULL,
    num_jobs_running bigint DEFAULT 0 NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT name_length CHECK (((char_length(name) > 0) AND (char_length(name) < 128))),
    CONSTRAINT num_jobs_completed_zero_or_positive CHECK ((num_jobs_completed >= 0)),
    CONSTRAINT num_jobs_running_zero_or_positive CHECK ((num_jobs_running >= 0))
);


--
-- Name: river_job; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.river_job (
    id bigint NOT NULL,
    state public.river_job_state DEFAULT 'available'::public.river_job_state NOT NULL,
    attempt smallint DEFAULT 0 NOT NULL,
    max_attempts smallint NOT NULL,
    attempted_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    finalized_at timestamp with time zone,
    scheduled_at timestamp with time zone DEFAULT now() NOT NULL,
    priority smallint DEFAULT 1 NOT NULL,
    args jsonb NOT NULL,
    attempted_by text[],
    errors jsonb[],
    kind text NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    queue text DEFAULT 'default'::text NOT NULL,
    tags character varying(255)[] DEFAULT '{}'::character varying[] NOT NULL,
    unique_key bytea,
    unique_states bit(8),
    CONSTRAINT finalized_or_finalized_at_null CHECK ((((finalized_at IS NULL) AND (state <> ALL (ARRAY['cancelled'::public.river_job_state, 'completed'::public.river_job_state, 'discarded'::public.river_job_state]))) OR ((finalized_at IS NOT NULL) AND (state = ANY (ARRAY['cancelled'::public.river_job_state, 'completed'::public.river_job_state, 'discarded'::public.river_job_state]))))),
    CONSTRAINT kind_length CHECK (((char_length(kind) > 0) AND (char_length(kind) < 128))),
    CONSTRAINT max_attempts_is_positive CHECK ((max_attempts > 0)),
    CONSTRAINT priority_in_range CHECK (((priority >= 1) AND (priority <= 4))),
    CONSTRAINT queue_length CHECK (((char_length(queue) > 0) AND (char_length(queue) < 128)))
);


--
-- Name: river_job_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.river_job_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: river_job_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.river_job_id_seq OWNED BY public.river_job.id;


--
-- Name: river_leader; Type: TABLE; Schema: public; Owner: -
--

CREATE UNLOGGED TABLE public.river_leader (
    elected_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    leader_id text NOT NULL,
    name text DEFAULT 'default'::text NOT NULL,
    CONSTRAINT leader_id_length CHECK (((char_length(leader_id) > 0) AND (char_length(leader_id) < 128))),
    CONSTRAINT name_length CHECK ((name = 'default'::text))
);


--
-- Name: river_migration; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.river_migration (
    line text NOT NULL,
    version bigint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT line_length CHECK (((char_length(line) > 0) AND (char_length(line) < 128))),
    CONSTRAINT version_gte_1 CHECK ((version >= 1))
);


--
-- Name: river_queue; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.river_queue (
    name text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    paused_at timestamp with time zone,
    updated_at timestamp with time zone NOT NULL
);


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
-- Name: subscription_payments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.subscription_payments (
    id bigint NOT NULL,
    subscription_id bigint,
    razorpay_payment_id character varying(255) NOT NULL,
    razorpay_order_id character varying(255),
    razorpay_invoice_id character varying(255),
    amount bigint NOT NULL,
    currency character varying(10) DEFAULT 'INR'::character varying NOT NULL,
    status character varying(50) NOT NULL,
    method character varying(50),
    authorized_at timestamp with time zone,
    captured_at timestamp with time zone,
    failed_at timestamp with time zone,
    refunded_at timestamp with time zone,
    razorpay_data jsonb,
    error_code character varying(100),
    error_description text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    captured boolean DEFAULT false NOT NULL
);


--
-- Name: TABLE subscription_payments; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.subscription_payments IS 'Complete history of all payments for subscriptions';


--
-- Name: COLUMN subscription_payments.amount; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.subscription_payments.amount IS 'Amount in smallest currency unit (paise for INR)';


--
-- Name: COLUMN subscription_payments.status; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.subscription_payments.status IS 'Payment status: authorized, captured, failed, refunded';


--
-- Name: COLUMN subscription_payments.captured; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.subscription_payments.captured IS 'Whether the payment has been captured (true) or just authorized (false)';


--
-- Name: subscription_payments_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.subscription_payments_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: subscription_payments_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.subscription_payments_id_seq OWNED BY public.subscription_payments.id;


--
-- Name: subscriptions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.subscriptions (
    id bigint NOT NULL,
    razorpay_subscription_id character varying(255) NOT NULL,
    razorpay_plan_id character varying(255) NOT NULL,
    owner_user_id bigint NOT NULL,
    plan_type character varying(50) NOT NULL,
    quantity integer NOT NULL,
    assigned_seats integer DEFAULT 0 NOT NULL,
    status character varying(50) NOT NULL,
    current_period_start timestamp with time zone,
    current_period_end timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    activated_at timestamp with time zone,
    cancelled_at timestamp with time zone,
    expired_at timestamp with time zone,
    razorpay_data jsonb,
    org_id bigint,
    license_expires_at timestamp with time zone,
    notes jsonb,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    last_payment_id character varying(255),
    last_payment_status character varying(50),
    last_payment_received_at timestamp with time zone,
    payment_verified boolean DEFAULT false NOT NULL,
    cancel_at_period_end boolean DEFAULT false,
    short_url character varying(500),
    CONSTRAINT valid_assigned_seats CHECK (((assigned_seats >= 0) AND (assigned_seats <= quantity))),
    CONSTRAINT valid_quantity CHECK ((quantity > 0))
);


--
-- Name: COLUMN subscriptions.last_payment_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.subscriptions.last_payment_id IS 'Razorpay payment ID from most recent payment';


--
-- Name: COLUMN subscriptions.last_payment_status; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.subscriptions.last_payment_status IS 'Status of last payment: authorized, captured, failed, refunded';


--
-- Name: COLUMN subscriptions.last_payment_received_at; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.subscriptions.last_payment_received_at IS 'Timestamp when payment was actually received (captured)';


--
-- Name: COLUMN subscriptions.payment_verified; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.subscriptions.payment_verified IS 'Whether any payment has been successfully received for this subscription';


--
-- Name: COLUMN subscriptions.short_url; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.subscriptions.short_url IS 'Razorpay public link for customers to manage subscription (no login required)';


--
-- Name: subscriptions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.subscriptions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: subscriptions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.subscriptions_id_seq OWNED BY public.subscriptions.id;


--
-- Name: system_default_ai_configs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.system_default_ai_configs (
    id integer NOT NULL,
    tier_name character varying(64) NOT NULL,
    provider_name character varying(64) NOT NULL,
    model_name character varying(128) NOT NULL,
    master_api_key text NOT NULL,
    is_active boolean DEFAULT true,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: system_default_ai_configs_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.system_default_ai_configs_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: system_default_ai_configs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.system_default_ai_configs_id_seq OWNED BY public.system_default_ai_configs.id;


--
-- Name: trial_eligibility; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.trial_eligibility (
    id bigint NOT NULL,
    normalized_email character varying(255) NOT NULL,
    first_user_id bigint,
    first_org_id bigint,
    first_subscription_id bigint,
    first_plan_code character varying(64),
    reservation_token character varying(128),
    reservation_expires_at timestamp with time zone,
    reserved_user_id bigint,
    reserved_org_id bigint,
    reserved_plan_code character varying(64),
    consumed boolean DEFAULT false NOT NULL,
    consumed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT chk_trial_eligibility_consumed_window CHECK ((((consumed = true) AND (consumed_at IS NOT NULL)) OR (consumed = false))),
    CONSTRAINT chk_trial_eligibility_email_lowercase CHECK (((normalized_email)::text = lower((normalized_email)::text))),
    CONSTRAINT chk_trial_eligibility_reservation_pair CHECK ((((reservation_token IS NULL) AND (reservation_expires_at IS NULL)) OR ((reservation_token IS NOT NULL) AND (reservation_expires_at IS NOT NULL))))
);


--
-- Name: trial_eligibility_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.trial_eligibility_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: trial_eligibility_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.trial_eligibility_id_seq OWNED BY public.trial_eligibility.id;


--
-- Name: upgrade_payment_attempts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.upgrade_payment_attempts (
    id bigint NOT NULL,
    org_id bigint NOT NULL,
    preview_token_sha256 character(64) NOT NULL,
    from_plan_code character varying(64) NOT NULL,
    to_plan_code character varying(64) NOT NULL,
    amount_cents bigint NOT NULL,
    currency character varying(16) NOT NULL,
    razorpay_mode character varying(16) NOT NULL,
    razorpay_order_id character varying(255) NOT NULL,
    razorpay_payment_id character varying(255),
    status character varying(64) DEFAULT 'prepared'::character varying NOT NULL,
    execute_idempotency_key character varying(255),
    execute_response jsonb,
    error_code character varying(128),
    error_reason character varying(255),
    error_description text,
    error_source character varying(128),
    error_step character varying(128),
    prepared_at timestamp with time zone DEFAULT now() NOT NULL,
    payment_failed_at timestamp with time zone,
    payment_captured_at timestamp with time zone,
    executed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    upgrade_request_id character varying(36),
    CONSTRAINT chk_upgrade_payment_attempts_amount_non_negative CHECK ((amount_cents >= 0)),
    CONSTRAINT chk_upgrade_payment_attempts_status CHECK (((status)::text = ANY ((ARRAY['prepared'::character varying, 'payment_failed'::character varying, 'payment_captured'::character varying, 'execute_applied'::character varying])::text[])))
);


--
-- Name: upgrade_payment_attempts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.upgrade_payment_attempts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: upgrade_payment_attempts_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.upgrade_payment_attempts_id_seq OWNED BY public.upgrade_payment_attempts.id;


--
-- Name: upgrade_replacement_cutovers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.upgrade_replacement_cutovers (
    id bigint NOT NULL,
    upgrade_request_id character varying(36) NOT NULL,
    org_id bigint NOT NULL,
    owner_user_id bigint NOT NULL,
    old_local_subscription_id bigint NOT NULL,
    old_razorpay_subscription_id character varying(255) NOT NULL,
    replacement_local_subscription_id bigint,
    replacement_razorpay_subscription_id character varying(255),
    target_plan_code character varying(64) NOT NULL,
    target_quantity integer NOT NULL,
    currency character varying(16) NOT NULL,
    cutover_at timestamp with time zone NOT NULL,
    old_cancellation_scheduled boolean DEFAULT false NOT NULL,
    status character varying(64) DEFAULT 'pending_provisioning'::character varying NOT NULL,
    retry_count integer DEFAULT 0 NOT NULL,
    next_retry_at timestamp with time zone,
    last_error text,
    last_attempted_at timestamp with time zone,
    resolved_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT chk_upgrade_replacement_cutovers_retry_non_negative CHECK ((retry_count >= 0)),
    CONSTRAINT chk_upgrade_replacement_cutovers_status CHECK (((status)::text = ANY ((ARRAY['pending_provisioning'::character varying, 'replacement_created'::character varying, 'old_cancellation_scheduled'::character varying, 'retry_pending'::character varying, 'manual_review_required'::character varying, 'completed'::character varying])::text[]))),
    CONSTRAINT chk_upgrade_replacement_cutovers_target_quantity_positive CHECK ((target_quantity > 0))
);


--
-- Name: upgrade_replacement_cutovers_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.upgrade_replacement_cutovers_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: upgrade_replacement_cutovers_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.upgrade_replacement_cutovers_id_seq OWNED BY public.upgrade_replacement_cutovers.id;


--
-- Name: upgrade_request_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.upgrade_request_events (
    id bigint NOT NULL,
    upgrade_request_id character varying(36) NOT NULL,
    org_id bigint NOT NULL,
    event_source character varying(64) NOT NULL,
    event_type character varying(64) NOT NULL,
    from_status character varying(64),
    to_status character varying(64),
    event_payload jsonb,
    event_time timestamp with time zone DEFAULT now() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: upgrade_request_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.upgrade_request_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: upgrade_request_events_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.upgrade_request_events_id_seq OWNED BY public.upgrade_request_events.id;


--
-- Name: upgrade_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.upgrade_requests (
    id bigint NOT NULL,
    upgrade_request_id character varying(36) NOT NULL,
    org_id bigint NOT NULL,
    actor_user_id bigint NOT NULL,
    from_plan_code character varying(64) NOT NULL,
    to_plan_code character varying(64) NOT NULL,
    expected_amount_cents bigint NOT NULL,
    currency character varying(16) NOT NULL,
    preview_token_sha256 character(64) NOT NULL,
    razorpay_mode character varying(16),
    razorpay_order_id character varying(255),
    razorpay_payment_id character varying(255),
    local_subscription_id bigint,
    razorpay_subscription_id character varying(255),
    target_quantity integer,
    payment_capture_confirmed boolean DEFAULT false NOT NULL,
    payment_capture_confirmed_at timestamp with time zone,
    subscription_change_confirmed boolean DEFAULT false NOT NULL,
    subscription_change_confirmed_at timestamp with time zone,
    plan_grant_applied boolean DEFAULT false NOT NULL,
    plan_grant_applied_at timestamp with time zone,
    current_status character varying(64) DEFAULT 'created'::character varying NOT NULL,
    failure_reason text,
    resolved_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    customer_state character varying(64),
    action_needed_at timestamp with time zone,
    last_customer_state_change_at timestamp with time zone,
    CONSTRAINT chk_upgrade_requests_amount_non_negative CHECK ((expected_amount_cents >= 0)),
    CONSTRAINT chk_upgrade_requests_customer_state CHECK (((customer_state IS NULL) OR ((customer_state)::text = ANY ((ARRAY['processing'::character varying, 'action_needed'::character varying, 'resolved'::character varying, 'failed'::character varying])::text[])))),
    CONSTRAINT chk_upgrade_requests_status CHECK (((current_status)::text = ANY ((ARRAY['created'::character varying, 'payment_order_created'::character varying, 'waiting_for_capture'::character varying, 'payment_capture_confirmed'::character varying, 'subscription_update_requested'::character varying, 'waiting_for_subscription_confirm'::character varying, 'subscription_change_confirmed'::character varying, 'reconciliation_retrying'::character varying, 'manual_review_required'::character varying, 'resolved'::character varying, 'failed'::character varying])::text[])))
);


--
-- Name: upgrade_requests_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.upgrade_requests_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: upgrade_requests_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.upgrade_requests_id_seq OWNED BY public.upgrade_requests.id;


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
    updated_at timestamp with time zone DEFAULT now(),
    plan_type character varying(50) DEFAULT 'free'::character varying NOT NULL,
    license_expires_at timestamp with time zone,
    active_subscription_id bigint
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
    password_reset_required boolean DEFAULT false NOT NULL,
    onboarding_api_key text,
    last_cli_used_at timestamp with time zone
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
-- Name: api_keys id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_keys ALTER COLUMN id SET DEFAULT nextval('public.api_keys_id_seq'::regclass);


--
-- Name: auth_tokens id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.auth_tokens ALTER COLUMN id SET DEFAULT nextval('public.auth_tokens_id_seq'::regclass);


--
-- Name: billing_notification_outbox id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.billing_notification_outbox ALTER COLUMN id SET DEFAULT nextval('public.billing_notification_outbox_id_seq'::regclass);


--
-- Name: instance_details id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.instance_details ALTER COLUMN id SET DEFAULT nextval('public.instance_details_id_seq'::regclass);


--
-- Name: integration_tokens id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.integration_tokens ALTER COLUMN id SET DEFAULT nextval('public.integration_tokens_id_seq'::regclass);


--
-- Name: license_log id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.license_log ALTER COLUMN id SET DEFAULT nextval('public.license_log_id_seq'::regclass);


--
-- Name: license_seat_assignments id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.license_seat_assignments ALTER COLUMN id SET DEFAULT nextval('public.license_seat_assignments_id_seq'::regclass);


--
-- Name: loc_lifecycle_log id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.loc_lifecycle_log ALTER COLUMN id SET DEFAULT nextval('public.loc_lifecycle_log_id_seq'::regclass);


--
-- Name: loc_usage_ledger id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.loc_usage_ledger ALTER COLUMN id SET DEFAULT nextval('public.loc_usage_ledger_id_seq'::regclass);


--
-- Name: org_billing_state id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.org_billing_state ALTER COLUMN id SET DEFAULT nextval('public.org_billing_state_id_seq'::regclass);


--
-- Name: orgs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.orgs ALTER COLUMN id SET DEFAULT nextval('public.orgs_id_seq'::regclass);


--
-- Name: plan_catalog id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plan_catalog ALTER COLUMN id SET DEFAULT nextval('public.plan_catalog_id_seq'::regclass);


--
-- Name: prompt_application_context id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prompt_application_context ALTER COLUMN id SET DEFAULT nextval('public.prompt_application_context_id_seq'::regclass);


--
-- Name: prompt_chunks id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prompt_chunks ALTER COLUMN id SET DEFAULT nextval('public.prompt_chunks_id_seq'::regclass);


--
-- Name: quota_batch_settlements id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.quota_batch_settlements ALTER COLUMN id SET DEFAULT nextval('public.quota_batch_settlements_id_seq'::regclass);


--
-- Name: quota_operation_aggregates id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.quota_operation_aggregates ALTER COLUMN id SET DEFAULT nextval('public.quota_operation_aggregates_id_seq'::regclass);


--
-- Name: quota_policy_catalog id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.quota_policy_catalog ALTER COLUMN id SET DEFAULT nextval('public.quota_policy_catalog_id_seq'::regclass);


--
-- Name: recent_activity id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.recent_activity ALTER COLUMN id SET DEFAULT nextval('public.recent_activity_id_seq'::regclass);


--
-- Name: review_events id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.review_events ALTER COLUMN id SET DEFAULT nextval('public.review_events_id_seq'::regclass);


--
-- Name: reviews id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reviews ALTER COLUMN id SET DEFAULT nextval('public.reviews_id_seq'::regclass);


--
-- Name: river_job id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.river_job ALTER COLUMN id SET DEFAULT nextval('public.river_job_id_seq'::regclass);


--
-- Name: roles id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.roles ALTER COLUMN id SET DEFAULT nextval('public.roles_id_seq'::regclass);


--
-- Name: subscription_payments id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subscription_payments ALTER COLUMN id SET DEFAULT nextval('public.subscription_payments_id_seq'::regclass);


--
-- Name: subscriptions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subscriptions ALTER COLUMN id SET DEFAULT nextval('public.subscriptions_id_seq'::regclass);


--
-- Name: system_default_ai_configs id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.system_default_ai_configs ALTER COLUMN id SET DEFAULT nextval('public.system_default_ai_configs_id_seq'::regclass);


--
-- Name: trial_eligibility id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.trial_eligibility ALTER COLUMN id SET DEFAULT nextval('public.trial_eligibility_id_seq'::regclass);


--
-- Name: upgrade_payment_attempts id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.upgrade_payment_attempts ALTER COLUMN id SET DEFAULT nextval('public.upgrade_payment_attempts_id_seq'::regclass);


--
-- Name: upgrade_replacement_cutovers id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.upgrade_replacement_cutovers ALTER COLUMN id SET DEFAULT nextval('public.upgrade_replacement_cutovers_id_seq'::regclass);


--
-- Name: upgrade_request_events id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.upgrade_request_events ALTER COLUMN id SET DEFAULT nextval('public.upgrade_request_events_id_seq'::regclass);


--
-- Name: upgrade_requests id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.upgrade_requests ALTER COLUMN id SET DEFAULT nextval('public.upgrade_requests_id_seq'::regclass);


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
-- Name: api_keys api_keys_key_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_keys
    ADD CONSTRAINT api_keys_key_hash_key UNIQUE (key_hash);


--
-- Name: api_keys api_keys_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_keys
    ADD CONSTRAINT api_keys_pkey PRIMARY KEY (id);


--
-- Name: auth_tokens auth_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.auth_tokens
    ADD CONSTRAINT auth_tokens_pkey PRIMARY KEY (id);


--
-- Name: billing_notification_outbox billing_notification_outbox_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.billing_notification_outbox
    ADD CONSTRAINT billing_notification_outbox_pkey PRIMARY KEY (id);


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
-- Name: learning_events learning_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.learning_events
    ADD CONSTRAINT learning_events_pkey PRIMARY KEY (id);


--
-- Name: learnings learnings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.learnings
    ADD CONSTRAINT learnings_pkey PRIMARY KEY (id);


--
-- Name: learnings learnings_short_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.learnings
    ADD CONSTRAINT learnings_short_id_key UNIQUE (short_id);


--
-- Name: license_log license_log_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.license_log
    ADD CONSTRAINT license_log_pkey PRIMARY KEY (id);


--
-- Name: license_log license_log_razorpay_event_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.license_log
    ADD CONSTRAINT license_log_razorpay_event_id_key UNIQUE (razorpay_event_id);


--
-- Name: license_seat_assignments license_seat_assignments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.license_seat_assignments
    ADD CONSTRAINT license_seat_assignments_pkey PRIMARY KEY (id);


--
-- Name: license_state license_state_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.license_state
    ADD CONSTRAINT license_state_pkey PRIMARY KEY (id);


--
-- Name: loc_lifecycle_log loc_lifecycle_log_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.loc_lifecycle_log
    ADD CONSTRAINT loc_lifecycle_log_pkey PRIMARY KEY (id);


--
-- Name: loc_usage_ledger loc_usage_ledger_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.loc_usage_ledger
    ADD CONSTRAINT loc_usage_ledger_pkey PRIMARY KEY (id);


--
-- Name: org_billing_state org_billing_state_org_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.org_billing_state
    ADD CONSTRAINT org_billing_state_org_id_key UNIQUE (org_id);


--
-- Name: org_billing_state org_billing_state_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.org_billing_state
    ADD CONSTRAINT org_billing_state_pkey PRIMARY KEY (id);


--
-- Name: orgs orgs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.orgs
    ADD CONSTRAINT orgs_pkey PRIMARY KEY (id);


--
-- Name: plan_catalog plan_catalog_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plan_catalog
    ADD CONSTRAINT plan_catalog_pkey PRIMARY KEY (id);


--
-- Name: plan_catalog plan_catalog_plan_code_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plan_catalog
    ADD CONSTRAINT plan_catalog_plan_code_key UNIQUE (plan_code);


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
-- Name: quota_batch_settlements quota_batch_settlements_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.quota_batch_settlements
    ADD CONSTRAINT quota_batch_settlements_pkey PRIMARY KEY (id);


--
-- Name: quota_operation_aggregates quota_operation_aggregates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.quota_operation_aggregates
    ADD CONSTRAINT quota_operation_aggregates_pkey PRIMARY KEY (id);


--
-- Name: quota_policy_catalog quota_policy_catalog_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.quota_policy_catalog
    ADD CONSTRAINT quota_policy_catalog_pkey PRIMARY KEY (id);


--
-- Name: recent_activity recent_activity_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.recent_activity
    ADD CONSTRAINT recent_activity_pkey PRIMARY KEY (id);


--
-- Name: review_events review_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.review_events
    ADD CONSTRAINT review_events_pkey PRIMARY KEY (id);


--
-- Name: reviews reviews_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reviews
    ADD CONSTRAINT reviews_pkey PRIMARY KEY (id);


--
-- Name: river_client river_client_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.river_client
    ADD CONSTRAINT river_client_pkey PRIMARY KEY (id);


--
-- Name: river_client_queue river_client_queue_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.river_client_queue
    ADD CONSTRAINT river_client_queue_pkey PRIMARY KEY (river_client_id, name);


--
-- Name: river_job river_job_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.river_job
    ADD CONSTRAINT river_job_pkey PRIMARY KEY (id);


--
-- Name: river_leader river_leader_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.river_leader
    ADD CONSTRAINT river_leader_pkey PRIMARY KEY (name);


--
-- Name: river_migration river_migration_pkey1; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.river_migration
    ADD CONSTRAINT river_migration_pkey1 PRIMARY KEY (line, version);


--
-- Name: river_queue river_queue_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.river_queue
    ADD CONSTRAINT river_queue_pkey PRIMARY KEY (name);


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
-- Name: subscription_payments subscription_payments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subscription_payments
    ADD CONSTRAINT subscription_payments_pkey PRIMARY KEY (id);


--
-- Name: subscription_payments subscription_payments_razorpay_payment_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subscription_payments
    ADD CONSTRAINT subscription_payments_razorpay_payment_id_key UNIQUE (razorpay_payment_id);


--
-- Name: subscriptions subscriptions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subscriptions
    ADD CONSTRAINT subscriptions_pkey PRIMARY KEY (id);


--
-- Name: subscriptions subscriptions_razorpay_subscription_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subscriptions
    ADD CONSTRAINT subscriptions_razorpay_subscription_id_key UNIQUE (razorpay_subscription_id);


--
-- Name: system_default_ai_configs system_default_ai_configs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.system_default_ai_configs
    ADD CONSTRAINT system_default_ai_configs_pkey PRIMARY KEY (id);


--
-- Name: system_default_ai_configs system_default_ai_configs_tier_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.system_default_ai_configs
    ADD CONSTRAINT system_default_ai_configs_tier_name_key UNIQUE (tier_name);


--
-- Name: trial_eligibility trial_eligibility_normalized_email_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.trial_eligibility
    ADD CONSTRAINT trial_eligibility_normalized_email_key UNIQUE (normalized_email);


--
-- Name: trial_eligibility trial_eligibility_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.trial_eligibility
    ADD CONSTRAINT trial_eligibility_pkey PRIMARY KEY (id);


--
-- Name: upgrade_payment_attempts upgrade_payment_attempts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.upgrade_payment_attempts
    ADD CONSTRAINT upgrade_payment_attempts_pkey PRIMARY KEY (id);


--
-- Name: upgrade_payment_attempts upgrade_payment_attempts_razorpay_order_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.upgrade_payment_attempts
    ADD CONSTRAINT upgrade_payment_attempts_razorpay_order_id_key UNIQUE (razorpay_order_id);


--
-- Name: upgrade_replacement_cutovers upgrade_replacement_cutovers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.upgrade_replacement_cutovers
    ADD CONSTRAINT upgrade_replacement_cutovers_pkey PRIMARY KEY (id);


--
-- Name: upgrade_replacement_cutovers upgrade_replacement_cutovers_upgrade_request_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.upgrade_replacement_cutovers
    ADD CONSTRAINT upgrade_replacement_cutovers_upgrade_request_id_key UNIQUE (upgrade_request_id);


--
-- Name: upgrade_request_events upgrade_request_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.upgrade_request_events
    ADD CONSTRAINT upgrade_request_events_pkey PRIMARY KEY (id);


--
-- Name: upgrade_requests upgrade_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.upgrade_requests
    ADD CONSTRAINT upgrade_requests_pkey PRIMARY KEY (id);


--
-- Name: upgrade_requests upgrade_requests_upgrade_request_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.upgrade_requests
    ADD CONSTRAINT upgrade_requests_upgrade_request_id_key UNIQUE (upgrade_request_id);


--
-- Name: billing_notification_outbox uq_billing_notification_outbox_dedupe; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.billing_notification_outbox
    ADD CONSTRAINT uq_billing_notification_outbox_dedupe UNIQUE (channel, dedupe_key);


--
-- Name: loc_lifecycle_log uq_loc_lifecycle_org_event_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.loc_lifecycle_log
    ADD CONSTRAINT uq_loc_lifecycle_org_event_key UNIQUE (org_id, event_key);


--
-- Name: loc_usage_ledger uq_loc_usage_ledger_org_idempotency; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.loc_usage_ledger
    ADD CONSTRAINT uq_loc_usage_ledger_org_idempotency UNIQUE (org_id, idempotency_key);


--
-- Name: quota_batch_settlements uq_quota_batch_settlements_dedupe; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.quota_batch_settlements
    ADD CONSTRAINT uq_quota_batch_settlements_dedupe UNIQUE (org_id, idempotency_key, batch_index);


--
-- Name: quota_operation_aggregates uq_quota_operation_aggregates_dedupe; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.quota_operation_aggregates
    ADD CONSTRAINT uq_quota_operation_aggregates_dedupe UNIQUE (org_id, idempotency_key);


--
-- Name: quota_policy_catalog uq_quota_policy_catalog_plan_provider; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.quota_policy_catalog
    ADD CONSTRAINT uq_quota_policy_catalog_plan_provider UNIQUE (plan_code, provider_key);


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
-- Name: idx_api_keys_key_hash; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_api_keys_key_hash ON public.api_keys USING btree (key_hash);


--
-- Name: idx_api_keys_key_prefix; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_api_keys_key_prefix ON public.api_keys USING btree (key_prefix);


--
-- Name: idx_api_keys_org_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_api_keys_org_id ON public.api_keys USING btree (org_id);


--
-- Name: idx_api_keys_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_api_keys_user_id ON public.api_keys USING btree (user_id);


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
-- Name: idx_billing_notification_outbox_org_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_billing_notification_outbox_org_created ON public.billing_notification_outbox USING btree (org_id, created_at DESC);


--
-- Name: idx_billing_notification_outbox_pending; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_billing_notification_outbox_pending ON public.billing_notification_outbox USING btree (status, send_after, created_at) WHERE ((status)::text = ANY ((ARRAY['pending'::character varying, 'failed'::character varying])::text[]));


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
-- Name: idx_learning_events_learning; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_learning_events_learning ON public.learning_events USING btree (learning_id, created_at DESC);


--
-- Name: idx_learning_events_org_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_learning_events_org_created ON public.learning_events USING btree (org_id, created_at DESC);


--
-- Name: idx_learnings_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_learnings_active ON public.learnings USING btree (org_id) WHERE (status = 'active'::public.learning_status);


--
-- Name: idx_learnings_org_simhash; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_learnings_org_simhash ON public.learnings USING btree (org_id, simhash);


--
-- Name: idx_learnings_tags; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_learnings_tags ON public.learnings USING gin (tags);


--
-- Name: idx_learnings_tsv; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_learnings_tsv ON public.learnings USING gin (tsv);


--
-- Name: idx_license_log_action; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_license_log_action ON public.license_log USING btree (event_type);


--
-- Name: idx_license_log_processed; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_license_log_processed ON public.license_log USING btree (processed) WHERE (processed = false);


--
-- Name: idx_license_log_razorpay; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_license_log_razorpay ON public.license_log USING btree (razorpay_event_id) WHERE (razorpay_event_id IS NOT NULL);


--
-- Name: idx_license_log_subscription; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_license_log_subscription ON public.license_log USING btree (subscription_id);


--
-- Name: idx_license_log_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_license_log_user ON public.license_log USING btree (user_id);


--
-- Name: idx_license_seat_assignments_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_license_seat_assignments_active ON public.license_seat_assignments USING btree (is_active) WHERE (is_active = true);


--
-- Name: idx_license_seat_assignments_assigned_by; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_license_seat_assignments_assigned_by ON public.license_seat_assignments USING btree (assigned_by_user_id);


--
-- Name: idx_license_seat_assignments_user_active; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_license_seat_assignments_user_active ON public.license_seat_assignments USING btree (user_id) WHERE (is_active = true);


--
-- Name: idx_license_state_expires_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_license_state_expires_at ON public.license_state USING btree (expires_at);


--
-- Name: idx_license_state_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_license_state_status ON public.license_state USING btree (status);


--
-- Name: idx_loc_lifecycle_log_email_pending; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_loc_lifecycle_log_email_pending ON public.loc_lifecycle_log USING btree (notified_email, created_at) WHERE (notified_email = false);


--
-- Name: idx_loc_lifecycle_log_event_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_loc_lifecycle_log_event_type ON public.loc_lifecycle_log USING btree (event_type);


--
-- Name: idx_loc_lifecycle_log_org_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_loc_lifecycle_log_org_created ON public.loc_lifecycle_log USING btree (org_id, created_at DESC);


--
-- Name: idx_loc_usage_ledger_operation; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_loc_usage_ledger_operation ON public.loc_usage_ledger USING btree (operation_type, trigger_source);


--
-- Name: idx_loc_usage_ledger_org_accounted_tokens; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_loc_usage_ledger_org_accounted_tokens ON public.loc_usage_ledger USING btree (org_id, accounted_at DESC) WHERE ((input_tokens IS NOT NULL) OR (output_tokens IS NOT NULL) OR (llm_cost_usd IS NOT NULL));


--
-- Name: idx_loc_usage_ledger_org_period_user_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_loc_usage_ledger_org_period_user_time ON public.loc_usage_ledger USING btree (org_id, billing_period_start, user_id, accounted_at DESC) WHERE ((status)::text = 'accounted'::text);


--
-- Name: idx_loc_usage_ledger_org_review; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_loc_usage_ledger_org_review ON public.loc_usage_ledger USING btree (org_id, review_id) WHERE (review_id IS NOT NULL);


--
-- Name: idx_loc_usage_ledger_org_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_loc_usage_ledger_org_time ON public.loc_usage_ledger USING btree (org_id, accounted_at DESC);


--
-- Name: idx_loc_usage_ledger_org_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_loc_usage_ledger_org_user ON public.loc_usage_ledger USING btree (org_id, user_id) WHERE (user_id IS NOT NULL);


--
-- Name: idx_org_billing_current_plan; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_org_billing_current_plan ON public.org_billing_state USING btree (current_plan_code);


--
-- Name: idx_org_billing_scheduled_effective; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_org_billing_scheduled_effective ON public.org_billing_state USING btree (scheduled_plan_effective_at) WHERE (scheduled_plan_effective_at IS NOT NULL);


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
-- Name: idx_plan_catalog_active_rank; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_plan_catalog_active_rank ON public.plan_catalog USING btree (active, rank);


--
-- Name: idx_quota_batch_settlements_org_idempotency; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_quota_batch_settlements_org_idempotency ON public.quota_batch_settlements USING btree (org_id, idempotency_key);


--
-- Name: idx_quota_batch_settlements_org_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_quota_batch_settlements_org_time ON public.quota_batch_settlements USING btree (org_id, accounted_at DESC);


--
-- Name: idx_quota_operation_aggregates_org_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_quota_operation_aggregates_org_time ON public.quota_operation_aggregates USING btree (org_id, finalized_at DESC);


--
-- Name: idx_quota_policy_catalog_lookup; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_quota_policy_catalog_lookup ON public.quota_policy_catalog USING btree (plan_code, provider_key) WHERE (active = true);


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
-- Name: idx_review_events_org_ts; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_review_events_org_ts ON public.review_events USING btree (org_id, ts);


--
-- Name: idx_review_events_review_ts; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_review_events_review_ts ON public.review_events USING btree (review_id, ts);


--
-- Name: idx_review_events_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_review_events_type ON public.review_events USING btree (review_id, event_type, ts DESC);


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
-- Name: idx_subscription_payments_captured; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_subscription_payments_captured ON public.subscription_payments USING btree (captured_at) WHERE (captured_at IS NOT NULL);


--
-- Name: idx_subscription_payments_captured_bool; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_subscription_payments_captured_bool ON public.subscription_payments USING btree (captured);


--
-- Name: idx_subscription_payments_razorpay; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_subscription_payments_razorpay ON public.subscription_payments USING btree (razorpay_payment_id);


--
-- Name: idx_subscription_payments_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_subscription_payments_status ON public.subscription_payments USING btree (status);


--
-- Name: idx_subscription_payments_subscription; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_subscription_payments_subscription ON public.subscription_payments USING btree (subscription_id);


--
-- Name: idx_subscriptions_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_subscriptions_org ON public.subscriptions USING btree (org_id);


--
-- Name: idx_subscriptions_owner; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_subscriptions_owner ON public.subscriptions USING btree (owner_user_id);


--
-- Name: idx_subscriptions_payment_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_subscriptions_payment_status ON public.subscriptions USING btree (last_payment_status);


--
-- Name: idx_subscriptions_payment_verified; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_subscriptions_payment_verified ON public.subscriptions USING btree (payment_verified);


--
-- Name: idx_subscriptions_razorpay; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_subscriptions_razorpay ON public.subscriptions USING btree (razorpay_subscription_id);


--
-- Name: idx_subscriptions_short_url; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_subscriptions_short_url ON public.subscriptions USING btree (short_url) WHERE (short_url IS NOT NULL);


--
-- Name: idx_subscriptions_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_subscriptions_status ON public.subscriptions USING btree (status);


--
-- Name: idx_trial_eligibility_consumed; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_trial_eligibility_consumed ON public.trial_eligibility USING btree (consumed, consumed_at DESC);


--
-- Name: idx_trial_eligibility_reservation_expires; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_trial_eligibility_reservation_expires ON public.trial_eligibility USING btree (reservation_expires_at) WHERE (reservation_expires_at IS NOT NULL);


--
-- Name: idx_upgrade_payment_attempts_execute_key; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_upgrade_payment_attempts_execute_key ON public.upgrade_payment_attempts USING btree (execute_idempotency_key) WHERE (execute_idempotency_key IS NOT NULL);


--
-- Name: idx_upgrade_payment_attempts_order; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_upgrade_payment_attempts_order ON public.upgrade_payment_attempts USING btree (razorpay_order_id);


--
-- Name: idx_upgrade_payment_attempts_org_preview; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_upgrade_payment_attempts_org_preview ON public.upgrade_payment_attempts USING btree (org_id, preview_token_sha256);


--
-- Name: idx_upgrade_payment_attempts_payment; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_upgrade_payment_attempts_payment ON public.upgrade_payment_attempts USING btree (razorpay_payment_id) WHERE (razorpay_payment_id IS NOT NULL);


--
-- Name: idx_upgrade_payment_attempts_request; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_upgrade_payment_attempts_request ON public.upgrade_payment_attempts USING btree (upgrade_request_id) WHERE (upgrade_request_id IS NOT NULL);


--
-- Name: idx_upgrade_payment_attempts_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_upgrade_payment_attempts_status ON public.upgrade_payment_attempts USING btree (status);


--
-- Name: idx_upgrade_replacement_cutovers_cutover_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_upgrade_replacement_cutovers_cutover_at ON public.upgrade_replacement_cutovers USING btree (cutover_at, status);


--
-- Name: idx_upgrade_replacement_cutovers_next_retry; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_upgrade_replacement_cutovers_next_retry ON public.upgrade_replacement_cutovers USING btree (next_retry_at) WHERE (next_retry_at IS NOT NULL);


--
-- Name: idx_upgrade_replacement_cutovers_org_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_upgrade_replacement_cutovers_org_status ON public.upgrade_replacement_cutovers USING btree (org_id, status, updated_at DESC);


--
-- Name: idx_upgrade_request_events_org_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_upgrade_request_events_org_time ON public.upgrade_request_events USING btree (org_id, event_time DESC);


--
-- Name: idx_upgrade_request_events_request_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_upgrade_request_events_request_time ON public.upgrade_request_events USING btree (upgrade_request_id, event_time DESC);


--
-- Name: idx_upgrade_requests_customer_state; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_upgrade_requests_customer_state ON public.upgrade_requests USING btree (org_id, customer_state, updated_at DESC) WHERE (customer_state IS NOT NULL);


--
-- Name: idx_upgrade_requests_order; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_upgrade_requests_order ON public.upgrade_requests USING btree (razorpay_order_id) WHERE (razorpay_order_id IS NOT NULL);


--
-- Name: idx_upgrade_requests_org_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_upgrade_requests_org_created ON public.upgrade_requests USING btree (org_id, created_at DESC);


--
-- Name: idx_upgrade_requests_org_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_upgrade_requests_org_status ON public.upgrade_requests USING btree (org_id, current_status, updated_at DESC);


--
-- Name: idx_upgrade_requests_payment; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_upgrade_requests_payment ON public.upgrade_requests USING btree (razorpay_payment_id) WHERE (razorpay_payment_id IS NOT NULL);


--
-- Name: idx_upgrade_requests_pending_apply; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_upgrade_requests_pending_apply ON public.upgrade_requests USING btree (current_status, plan_grant_applied, updated_at) WHERE (((current_status)::text = 'resolved'::text) AND (plan_grant_applied = false));


--
-- Name: idx_upgrade_requests_subscription; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_upgrade_requests_subscription ON public.upgrade_requests USING btree (razorpay_subscription_id) WHERE (razorpay_subscription_id IS NOT NULL);


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
-- Name: idx_user_roles_license_expires; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_roles_license_expires ON public.user_roles USING btree (license_expires_at) WHERE (license_expires_at IS NOT NULL);


--
-- Name: idx_user_roles_org_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_roles_org_id ON public.user_roles USING btree (org_id);


--
-- Name: idx_user_roles_plan_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_roles_plan_type ON public.user_roles USING btree (plan_type);


--
-- Name: idx_user_roles_subscription; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_roles_subscription ON public.user_roles USING btree (active_subscription_id) WHERE (active_subscription_id IS NOT NULL);


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
-- Name: idx_users_onboarding_api_key; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_onboarding_api_key ON public.users USING btree (onboarding_api_key) WHERE (onboarding_api_key IS NOT NULL);


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
-- Name: river_job_args_index; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX river_job_args_index ON public.river_job USING gin (args);


--
-- Name: river_job_kind; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX river_job_kind ON public.river_job USING btree (kind);


--
-- Name: river_job_metadata_index; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX river_job_metadata_index ON public.river_job USING gin (metadata);


--
-- Name: river_job_prioritized_fetching_index; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX river_job_prioritized_fetching_index ON public.river_job USING btree (state, queue, priority, scheduled_at, id);


--
-- Name: river_job_state_and_finalized_at_index; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX river_job_state_and_finalized_at_index ON public.river_job USING btree (state, finalized_at) WHERE (finalized_at IS NOT NULL);


--
-- Name: river_job_unique_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX river_job_unique_idx ON public.river_job USING btree (unique_key) WHERE ((unique_key IS NOT NULL) AND (unique_states IS NOT NULL) AND public.river_job_state_in_bitmask(unique_states, state));


--
-- Name: ux_license_state_singleton; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX ux_license_state_singleton ON public.license_state USING btree (id);


--
-- Name: license_seat_assignments trg_license_seat_assignments_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trg_license_seat_assignments_updated_at BEFORE UPDATE ON public.license_seat_assignments FOR EACH ROW EXECUTE FUNCTION public.license_seat_assignments_set_updated_at();


--
-- Name: license_state trg_license_state_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trg_license_state_updated_at BEFORE UPDATE ON public.license_state FOR EACH ROW EXECUTE FUNCTION public.license_state_set_updated_at();


--
-- Name: org_billing_state trg_org_billing_state_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trg_org_billing_state_updated_at BEFORE UPDATE ON public.org_billing_state FOR EACH ROW EXECUTE FUNCTION public.org_billing_state_set_updated_at();


--
-- Name: plan_catalog trg_plan_catalog_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trg_plan_catalog_updated_at BEFORE UPDATE ON public.plan_catalog FOR EACH ROW EXECUTE FUNCTION public.plan_catalog_set_updated_at();


--
-- Name: quota_batch_settlements trg_quota_batch_settlements_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trg_quota_batch_settlements_updated_at BEFORE UPDATE ON public.quota_batch_settlements FOR EACH ROW EXECUTE FUNCTION public.quota_batch_settlements_set_updated_at();


--
-- Name: quota_operation_aggregates trg_quota_operation_aggregates_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trg_quota_operation_aggregates_updated_at BEFORE UPDATE ON public.quota_operation_aggregates FOR EACH ROW EXECUTE FUNCTION public.quota_operation_aggregates_set_updated_at();


--
-- Name: quota_policy_catalog trg_quota_policy_catalog_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trg_quota_policy_catalog_updated_at BEFORE UPDATE ON public.quota_policy_catalog FOR EACH ROW EXECUTE FUNCTION public.quota_policy_catalog_set_updated_at();


--
-- Name: trial_eligibility trg_trial_eligibility_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trg_trial_eligibility_updated_at BEFORE UPDATE ON public.trial_eligibility FOR EACH ROW EXECUTE FUNCTION public.trial_eligibility_set_updated_at();


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
-- Name: api_keys api_keys_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_keys
    ADD CONSTRAINT api_keys_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;


--
-- Name: api_keys api_keys_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_keys
    ADD CONSTRAINT api_keys_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: auth_tokens auth_tokens_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.auth_tokens
    ADD CONSTRAINT auth_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: billing_notification_outbox billing_notification_outbox_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.billing_notification_outbox
    ADD CONSTRAINT billing_notification_outbox_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;


--
-- Name: billing_notification_outbox billing_notification_outbox_recipient_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.billing_notification_outbox
    ADD CONSTRAINT billing_notification_outbox_recipient_user_id_fkey FOREIGN KEY (recipient_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: dashboard_cache dashboard_cache_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dashboard_cache
    ADD CONSTRAINT dashboard_cache_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id);


--
-- Name: upgrade_payment_attempts fk_upgrade_payment_attempts_upgrade_request; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.upgrade_payment_attempts
    ADD CONSTRAINT fk_upgrade_payment_attempts_upgrade_request FOREIGN KEY (upgrade_request_id) REFERENCES public.upgrade_requests(upgrade_request_id) ON DELETE SET NULL;


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
-- Name: learning_events learning_events_learning_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.learning_events
    ADD CONSTRAINT learning_events_learning_id_fkey FOREIGN KEY (learning_id) REFERENCES public.learnings(id) ON DELETE CASCADE;


--
-- Name: license_log license_log_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.license_log
    ADD CONSTRAINT license_log_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.users(id);


--
-- Name: license_log license_log_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.license_log
    ADD CONSTRAINT license_log_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id);


--
-- Name: license_log license_log_subscription_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.license_log
    ADD CONSTRAINT license_log_subscription_id_fkey FOREIGN KEY (subscription_id) REFERENCES public.subscriptions(id);


--
-- Name: license_log license_log_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.license_log
    ADD CONSTRAINT license_log_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: license_seat_assignments license_seat_assignments_assigned_by_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.license_seat_assignments
    ADD CONSTRAINT license_seat_assignments_assigned_by_user_id_fkey FOREIGN KEY (assigned_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: license_seat_assignments license_seat_assignments_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.license_seat_assignments
    ADD CONSTRAINT license_seat_assignments_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: loc_lifecycle_log loc_lifecycle_log_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.loc_lifecycle_log
    ADD CONSTRAINT loc_lifecycle_log_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;


--
-- Name: loc_lifecycle_log loc_lifecycle_log_plan_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.loc_lifecycle_log
    ADD CONSTRAINT loc_lifecycle_log_plan_code_fkey FOREIGN KEY (plan_code) REFERENCES public.plan_catalog(plan_code);


--
-- Name: loc_lifecycle_log loc_lifecycle_log_usage_ledger_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.loc_lifecycle_log
    ADD CONSTRAINT loc_lifecycle_log_usage_ledger_id_fkey FOREIGN KEY (usage_ledger_id) REFERENCES public.loc_usage_ledger(id) ON DELETE SET NULL;


--
-- Name: loc_usage_ledger loc_usage_ledger_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.loc_usage_ledger
    ADD CONSTRAINT loc_usage_ledger_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;


--
-- Name: loc_usage_ledger loc_usage_ledger_review_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.loc_usage_ledger
    ADD CONSTRAINT loc_usage_ledger_review_id_fkey FOREIGN KEY (review_id) REFERENCES public.reviews(id) ON DELETE SET NULL;


--
-- Name: loc_usage_ledger loc_usage_ledger_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.loc_usage_ledger
    ADD CONSTRAINT loc_usage_ledger_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: org_billing_state org_billing_state_current_plan_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.org_billing_state
    ADD CONSTRAINT org_billing_state_current_plan_code_fkey FOREIGN KEY (current_plan_code) REFERENCES public.plan_catalog(plan_code);


--
-- Name: org_billing_state org_billing_state_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.org_billing_state
    ADD CONSTRAINT org_billing_state_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;


--
-- Name: org_billing_state org_billing_state_scheduled_plan_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.org_billing_state
    ADD CONSTRAINT org_billing_state_scheduled_plan_code_fkey FOREIGN KEY (scheduled_plan_code) REFERENCES public.plan_catalog(plan_code);


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
-- Name: quota_batch_settlements quota_batch_settlements_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.quota_batch_settlements
    ADD CONSTRAINT quota_batch_settlements_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;


--
-- Name: quota_batch_settlements quota_batch_settlements_plan_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.quota_batch_settlements
    ADD CONSTRAINT quota_batch_settlements_plan_code_fkey FOREIGN KEY (plan_code) REFERENCES public.plan_catalog(plan_code);


--
-- Name: quota_batch_settlements quota_batch_settlements_review_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.quota_batch_settlements
    ADD CONSTRAINT quota_batch_settlements_review_id_fkey FOREIGN KEY (review_id) REFERENCES public.reviews(id) ON DELETE SET NULL;


--
-- Name: quota_operation_aggregates quota_operation_aggregates_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.quota_operation_aggregates
    ADD CONSTRAINT quota_operation_aggregates_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;


--
-- Name: quota_operation_aggregates quota_operation_aggregates_plan_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.quota_operation_aggregates
    ADD CONSTRAINT quota_operation_aggregates_plan_code_fkey FOREIGN KEY (plan_code) REFERENCES public.plan_catalog(plan_code);


--
-- Name: quota_operation_aggregates quota_operation_aggregates_review_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.quota_operation_aggregates
    ADD CONSTRAINT quota_operation_aggregates_review_id_fkey FOREIGN KEY (review_id) REFERENCES public.reviews(id) ON DELETE SET NULL;


--
-- Name: quota_policy_catalog quota_policy_catalog_plan_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.quota_policy_catalog
    ADD CONSTRAINT quota_policy_catalog_plan_code_fkey FOREIGN KEY (plan_code) REFERENCES public.plan_catalog(plan_code) ON DELETE CASCADE;


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
-- Name: review_events review_events_review_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.review_events
    ADD CONSTRAINT review_events_review_id_fkey FOREIGN KEY (review_id) REFERENCES public.reviews(id) ON DELETE CASCADE;


--
-- Name: reviews reviews_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reviews
    ADD CONSTRAINT reviews_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id);


--
-- Name: river_client_queue river_client_queue_river_client_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.river_client_queue
    ADD CONSTRAINT river_client_queue_river_client_id_fkey FOREIGN KEY (river_client_id) REFERENCES public.river_client(id) ON DELETE CASCADE;


--
-- Name: subscription_payments subscription_payments_subscription_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subscription_payments
    ADD CONSTRAINT subscription_payments_subscription_id_fkey FOREIGN KEY (subscription_id) REFERENCES public.subscriptions(id);


--
-- Name: subscriptions subscriptions_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subscriptions
    ADD CONSTRAINT subscriptions_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id);


--
-- Name: subscriptions subscriptions_owner_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subscriptions
    ADD CONSTRAINT subscriptions_owner_user_id_fkey FOREIGN KEY (owner_user_id) REFERENCES public.users(id);


--
-- Name: trial_eligibility trial_eligibility_first_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.trial_eligibility
    ADD CONSTRAINT trial_eligibility_first_org_id_fkey FOREIGN KEY (first_org_id) REFERENCES public.orgs(id) ON DELETE SET NULL;


--
-- Name: trial_eligibility trial_eligibility_first_subscription_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.trial_eligibility
    ADD CONSTRAINT trial_eligibility_first_subscription_id_fkey FOREIGN KEY (first_subscription_id) REFERENCES public.subscriptions(id) ON DELETE SET NULL;


--
-- Name: trial_eligibility trial_eligibility_first_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.trial_eligibility
    ADD CONSTRAINT trial_eligibility_first_user_id_fkey FOREIGN KEY (first_user_id) REFERENCES public.users(id);


--
-- Name: trial_eligibility trial_eligibility_reserved_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.trial_eligibility
    ADD CONSTRAINT trial_eligibility_reserved_org_id_fkey FOREIGN KEY (reserved_org_id) REFERENCES public.orgs(id) ON DELETE SET NULL;


--
-- Name: trial_eligibility trial_eligibility_reserved_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.trial_eligibility
    ADD CONSTRAINT trial_eligibility_reserved_user_id_fkey FOREIGN KEY (reserved_user_id) REFERENCES public.users(id);


--
-- Name: upgrade_replacement_cutovers upgrade_replacement_cutovers_old_local_subscription_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.upgrade_replacement_cutovers
    ADD CONSTRAINT upgrade_replacement_cutovers_old_local_subscription_id_fkey FOREIGN KEY (old_local_subscription_id) REFERENCES public.subscriptions(id) ON DELETE RESTRICT;


--
-- Name: upgrade_replacement_cutovers upgrade_replacement_cutovers_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.upgrade_replacement_cutovers
    ADD CONSTRAINT upgrade_replacement_cutovers_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;


--
-- Name: upgrade_replacement_cutovers upgrade_replacement_cutovers_owner_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.upgrade_replacement_cutovers
    ADD CONSTRAINT upgrade_replacement_cutovers_owner_user_id_fkey FOREIGN KEY (owner_user_id) REFERENCES public.users(id) ON DELETE RESTRICT;


--
-- Name: upgrade_replacement_cutovers upgrade_replacement_cutovers_replacement_local_subscriptio_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.upgrade_replacement_cutovers
    ADD CONSTRAINT upgrade_replacement_cutovers_replacement_local_subscriptio_fkey FOREIGN KEY (replacement_local_subscription_id) REFERENCES public.subscriptions(id) ON DELETE SET NULL;


--
-- Name: upgrade_replacement_cutovers upgrade_replacement_cutovers_upgrade_request_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.upgrade_replacement_cutovers
    ADD CONSTRAINT upgrade_replacement_cutovers_upgrade_request_id_fkey FOREIGN KEY (upgrade_request_id) REFERENCES public.upgrade_requests(upgrade_request_id) ON DELETE CASCADE;


--
-- Name: upgrade_request_events upgrade_request_events_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.upgrade_request_events
    ADD CONSTRAINT upgrade_request_events_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;


--
-- Name: upgrade_request_events upgrade_request_events_upgrade_request_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.upgrade_request_events
    ADD CONSTRAINT upgrade_request_events_upgrade_request_id_fkey FOREIGN KEY (upgrade_request_id) REFERENCES public.upgrade_requests(upgrade_request_id) ON DELETE CASCADE;


--
-- Name: upgrade_requests upgrade_requests_actor_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.upgrade_requests
    ADD CONSTRAINT upgrade_requests_actor_user_id_fkey FOREIGN KEY (actor_user_id) REFERENCES public.users(id);


--
-- Name: upgrade_requests upgrade_requests_local_subscription_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.upgrade_requests
    ADD CONSTRAINT upgrade_requests_local_subscription_id_fkey FOREIGN KEY (local_subscription_id) REFERENCES public.subscriptions(id);


--
-- Name: upgrade_requests upgrade_requests_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.upgrade_requests
    ADD CONSTRAINT upgrade_requests_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;


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
-- Name: user_roles user_roles_active_subscription_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_roles
    ADD CONSTRAINT user_roles_active_subscription_id_fkey FOREIGN KEY (active_subscription_id) REFERENCES public.subscriptions(id);


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

\unrestrict dbmate


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
    ('20250905120000'),
    ('20250909120000'),
    ('20250924122125'),
    ('20250925120001'),
    ('20251007'),
    ('20251204105958'),
    ('20251204134413'),
    ('20251209'),
    ('20251213101233'),
    ('20251213103152'),
    ('20251213144431'),
    ('20251219135906'),
    ('20251222074428'),
    ('20251224132642'),
    ('20260120122547'),
    ('20260327100000'),
    ('20260327100100'),
    ('20260327100200'),
    ('20260327100300'),
    ('20260328121000'),
    ('20260328150000'),
    ('20260328151000'),
    ('20260330120000'),
    ('20260401153000'),
    ('20260401195429'),
    ('20260401204800'),
    ('20260403123000'),
    ('20260403124500'),
    ('20260403130000'),
    ('20260403151832'),
    ('20260411170000'),
    ('20260419193000'),
    ('20260420140334');
