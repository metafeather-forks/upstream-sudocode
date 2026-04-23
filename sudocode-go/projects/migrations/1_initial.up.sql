-- Specs table
CREATE TABLE specs (
    uuid         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id   TEXT NOT NULL,
    id           TEXT NOT NULL,
    title        TEXT NOT NULL DEFAULT '',
    file_path    TEXT NOT NULL DEFAULT '',
    content      TEXT NOT NULL DEFAULT '',
    priority     INT NOT NULL DEFAULT 2,
    archived     BOOLEAN NOT NULL DEFAULT false,
    archived_at  TIMESTAMPTZ,
    parent_id    TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, id)
);

CREATE INDEX idx_specs_project ON specs(project_id);

-- Issues table
CREATE TABLE issues (
    uuid         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id   TEXT NOT NULL,
    id           TEXT NOT NULL,
    title        TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'open',
    content      TEXT NOT NULL DEFAULT '',
    priority     INT NOT NULL DEFAULT 2,
    assignee     TEXT,
    archived     BOOLEAN NOT NULL DEFAULT false,
    archived_at  TIMESTAMPTZ,
    parent_id    TEXT,
    closed_at    TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, id)
);

CREATE INDEX idx_issues_project ON issues(project_id);
CREATE INDEX idx_issues_status ON issues(project_id, status);

-- Tags table (shared for specs and issues)
CREATE TABLE tags (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id   TEXT NOT NULL,
    entity_type  TEXT NOT NULL, -- 'spec' or 'issue'
    entity_id    TEXT NOT NULL,
    tag          TEXT NOT NULL,
    UNIQUE (project_id, entity_type, entity_id, tag)
);

CREATE INDEX idx_tags_entity ON tags(project_id, entity_type, entity_id);

-- Relationships table
CREATE TABLE relationships (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id        TEXT NOT NULL,
    from_id           TEXT NOT NULL,
    from_type         TEXT NOT NULL,
    to_id             TEXT NOT NULL,
    to_type           TEXT NOT NULL,
    relationship_type TEXT NOT NULL,
    metadata          JSONB,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, from_id, to_id, relationship_type)
);

CREATE INDEX idx_relationships_from ON relationships(project_id, from_id);
CREATE INDEX idx_relationships_to ON relationships(project_id, to_id);

-- Feedback table
CREATE TABLE feedback (
    uuid          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id    TEXT NOT NULL,
    id            TEXT NOT NULL,
    from_id       TEXT,
    to_id         TEXT NOT NULL,
    feedback_type TEXT NOT NULL DEFAULT 'comment',
    content       TEXT NOT NULL DEFAULT '',
    agent         TEXT,
    anchor        JSONB,
    dismissed     BOOLEAN NOT NULL DEFAULT false,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, id)
);

CREATE INDEX idx_feedback_to ON feedback(project_id, to_id);
