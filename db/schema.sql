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
-- Name: instance_details; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.instance_details (
    id integer NOT NULL,
    livereview_prod_url text NOT NULL,
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
    updated_at timestamp with time zone DEFAULT now() NOT NULL
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
-- Name: schema_migrations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.schema_migrations (
    version character varying NOT NULL
);


--
-- Name: instance_details id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.instance_details ALTER COLUMN id SET DEFAULT nextval('public.instance_details_id_seq'::regclass);


--
-- Name: integration_tokens id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.integration_tokens ALTER COLUMN id SET DEFAULT nextval('public.integration_tokens_id_seq'::regclass);


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
-- Name: schema_migrations schema_migrations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.schema_migrations
    ADD CONSTRAINT schema_migrations_pkey PRIMARY KEY (version);


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
    ('20250719000004');
