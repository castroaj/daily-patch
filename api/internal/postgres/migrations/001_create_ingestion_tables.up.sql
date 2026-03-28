-- 001_create_ingestion_tables.up.sql — creates the vulnerabilities and
-- ingestion_runs tables used by the ingestion pipeline.
--
-- These are the minimum tables required for end-to-end ingestion.
-- Scoring and newsletter tables will be added in a future migration.

CREATE TABLE IF NOT EXISTS vulnerabilities (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    source       TEXT        NOT NULL CHECK (source IN ('nvd', 'ghsa', 'exploitdb')),
    cve_id       TEXT        UNIQUE,
    ghsa_id      TEXT        UNIQUE,
    edb_id       TEXT        UNIQUE,
    title        TEXT        NOT NULL,
    description  TEXT,
    cvss_score   NUMERIC(4,1),
    cvss_vector  TEXT,
    published_at TIMESTAMPTZ,
    updated_at   TIMESTAMPTZ,
    raw_json     JSONB,
    ingested_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS ingestion_runs (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    source        TEXT        NOT NULL CHECK (source IN ('nvd', 'ghsa', 'exploitdb')),
    started_at    TIMESTAMPTZ NOT NULL,
    finished_at   TIMESTAMPTZ,
    items_fetched INTEGER     NOT NULL DEFAULT 0,
    items_new     INTEGER     NOT NULL DEFAULT 0
);
