CREATE TYPE membership_role AS ENUM ('owner', 'member');
CREATE TYPE membership_status AS ENUM ('pending', 'active');

CREATE TABLE memberships (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    identity_id     UUID NOT NULL,
    role            membership_role NOT NULL,
    status          membership_status NOT NULL,
    expires_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, identity_id)
);

CREATE INDEX idx_memberships_identity_id ON memberships (identity_id);
