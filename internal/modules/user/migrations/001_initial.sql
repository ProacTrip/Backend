-- +migrate Up
-- =============================================================================
-- CONSOLIDATED USER MODULE SCHEMA (migrations 001–012 merged)
-- Fresh DBs get everything in one atomic file. Deployed DBs use 013 for
-- incremental constraint additions.
-- =============================================================================

-- Shared utility trigger function
CREATE OR REPLACE FUNCTION update_updated_at_column()
    RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- =============================================================================
-- USER PROFILES
-- Aggregate root. user_id is cross-domain ref to Auth domain (no FK).
-- Created reactively when UserRegistered event is consumed.
-- email is denormalized from the registration event for query convenience.
-- =============================================================================
CREATE TABLE IF NOT EXISTS user_profiles (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    user_id UUID NOT NULL UNIQUE,
    email VARCHAR(255) NOT NULL,
    first_name VARCHAR(100),
    last_name VARCHAR(100),
    date_of_birth DATE,
    gender VARCHAR(20),
    nationality VARCHAR(100),
    phone VARCHAR(50),
    avatar_url TEXT,
    bio TEXT,
    language_code VARCHAR(5) NOT NULL DEFAULT 'es',
    currency_code VARCHAR(3) NOT NULL DEFAULT 'EUR',
    role VARCHAR(20) NOT NULL DEFAULT 'client',
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    ocr_populated BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_profiles_email UNIQUE (email),
    CONSTRAINT chk_gender CHECK (
        gender IS NULL OR gender IN ('male', 'female', 'non_binary', 'prefer_not_to_say')
    ),
    CONSTRAINT chk_user_role CHECK (role IN ('client', 'admin')),
    CONSTRAINT chk_profile_status CHECK (status IN ('active', 'inactive', 'suspended', 'deleted')),
    CONSTRAINT chk_phone_e164 CHECK (phone IS NULL OR phone ~ '^\+[1-9]\d{6,14}$')
);

CREATE TRIGGER trg_profiles_updated_at
    BEFORE UPDATE ON user_profiles
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE INDEX IF NOT EXISTS idx_profiles_user_id ON user_profiles (user_id);
CREATE INDEX IF NOT EXISTS idx_user_profiles_status ON user_profiles (status) WHERE status != 'active';

-- =============================================================================
-- USER TRAVEL PREFERENCES (1:1 with profile)
-- =============================================================================
CREATE TABLE IF NOT EXISTS user_travel_preferences (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    user_id UUID NOT NULL UNIQUE REFERENCES user_profiles (user_id) ON DELETE CASCADE,
    preferred_class VARCHAR(20) NOT NULL DEFAULT 'economy',
    seat_preference VARCHAR(20),
    meal_preference VARCHAR(50),
    special_assistance TEXT[],
    preferred_airlines UUID[],
    preferred_hotels TEXT[],
    avoid_layovers BOOLEAN NOT NULL DEFAULT FALSE,
    max_layover_duration INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_preferred_class CHECK (
        preferred_class IN ('economy', 'premium_economy', 'business', 'first')
    ),
    CONSTRAINT chk_seat_preference CHECK (
        seat_preference IS NULL OR
        seat_preference IN ('window', 'aisle', 'middle', 'no_preference')
    )
);

CREATE TRIGGER trg_travel_prefs_updated_at
    BEFORE UPDATE ON user_travel_preferences
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- =============================================================================
-- USER MEDICAL PROFILES (1:1 with profile)
-- JSONB-based with per-field encryption handled at the application layer.
-- =============================================================================
CREATE TABLE IF NOT EXISTS user_medical_profiles (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    user_id UUID NOT NULL UNIQUE REFERENCES user_profiles (user_id) ON DELETE CASCADE,
    data JSONB NOT NULL DEFAULT '{}',
    is_shared BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TRIGGER trg_medical_updated_at
    BEFORE UPDATE ON user_medical_profiles
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- =============================================================================
-- DOCUMENT TYPES (read-only catalog)
-- =============================================================================
CREATE TABLE IF NOT EXISTS document_types (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    code VARCHAR(50) NOT NULL UNIQUE,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    is_identity BOOLEAN NOT NULL DEFAULT FALSE,
    requires_ocr BOOLEAN NOT NULL DEFAULT FALSE,
    ocr_fields JSONB,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO document_types (code, name, description, is_identity, requires_ocr, is_active, sort_order)
VALUES
    ('passport',            'Passport',              'International passport',             true,  true,  true, 1),
    ('visa',                'Visa',                  'Travel visa document',               false, true,  true, 2),
    ('vaccination_cert',    'Vaccination Certificate','Health vaccination record',          false, true,  true, 3)
ON CONFLICT (code) DO NOTHING;

-- =============================================================================
-- USER DOCUMENTS
-- =============================================================================
CREATE TABLE IF NOT EXISTS user_documents (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    user_id UUID NOT NULL REFERENCES user_profiles(user_id) ON DELETE CASCADE,
    document_type_id UUID NOT NULL REFERENCES document_types(id),
    file_name VARCHAR(255) NOT NULL,
    file_size INTEGER,
    mime_type VARCHAR(100),
    storage_key TEXT NOT NULL,
    detected_mime_type VARCHAR(100),
    detected_size_bytes BIGINT,
    document_type VARCHAR(50),
    failure_reason TEXT,
    verification_status VARCHAR(20) NOT NULL DEFAULT 'unverified'
        CHECK (verification_status IN ('unverified', 'verified', 'rejected', 'manual_review', 'suspicious')),
    ocr_status VARCHAR(20) NOT NULL DEFAULT 'queued',
    ocr_data JSONB,
    ocr_confidence DOUBLE PRECISION,
    extracted_data JSONB,
    has_newer_medical_data BOOLEAN NOT NULL DEFAULT FALSE,
    medical_update_summary JSONB,
    valid_from TIMESTAMPTZ,
    valid_until TIMESTAMPTZ,
    document_number VARCHAR(100),
    issuing_country VARCHAR(100),
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_ocr_status CHECK (
        ocr_status IN ('queued', 'processing', 'validating', 'sanitizing', 'ocr_processing',
                       'completed', 'rejected', 'failed')
    )
);

CREATE TRIGGER trg_user_documents_updated_at
    BEFORE UPDATE ON user_documents
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE INDEX IF NOT EXISTS idx_user_documents_user_id ON user_documents (user_id);
CREATE INDEX IF NOT EXISTS idx_user_documents_type_id ON user_documents (document_type_id);
CREATE INDEX IF NOT EXISTS idx_user_documents_ocr_status ON user_documents (ocr_status);

-- =============================================================================
-- DOCUMENT VERIFICATION HISTORY (audit trail, admin-only writes)
-- =============================================================================
CREATE TABLE IF NOT EXISTS document_verification_history (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    document_id UUID NOT NULL REFERENCES user_documents(id),
    previous_status VARCHAR(20) NOT NULL,
    new_status VARCHAR(20) NOT NULL,
    verified_by UUID NOT NULL,
    reason TEXT,
    changed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_dvh_document_id ON document_verification_history(document_id);

-- =============================================================================
-- MEDICAL PENDING UPDATES
-- Conflicts detected when OCR or NLP extract data that differs from the
-- current medical profile. The user reviews and accepts/rejects/customizes.
-- =============================================================================
CREATE TABLE IF NOT EXISTS medical_pending_updates (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    user_id UUID NOT NULL REFERENCES user_profiles(user_id) ON DELETE CASCADE,
    source_type VARCHAR(10) NOT NULL,
    source_document_id UUID REFERENCES user_documents(id) ON DELETE CASCADE,
    conversation_id UUID,
    field_name VARCHAR(50) NOT NULL,
    current_value TEXT,
    proposed_value TEXT NOT NULL,
    suggested_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMPTZ NOT NULL DEFAULT (CURRENT_TIMESTAMP + interval '30 days'),
    status VARCHAR(10) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'accepted', 'rejected')),
    resolved_at TIMESTAMPTZ,
    CONSTRAINT chk_pending_source CHECK (
        (source_type = 'ocr' AND source_document_id IS NOT NULL) OR
        (source_type = 'nlp' AND conversation_id IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_medical_pending_user_status ON medical_pending_updates (user_id, status);
CREATE INDEX IF NOT EXISTS idx_medical_pending_expires ON medical_pending_updates (expires_at)
    WHERE status = 'pending';

-- +migrate Down
DROP TABLE IF EXISTS medical_pending_updates CASCADE;
DROP TABLE IF EXISTS document_verification_history CASCADE;
DROP TABLE IF EXISTS user_documents CASCADE;
DROP TABLE IF EXISTS document_types CASCADE;
DROP TABLE IF EXISTS user_medical_profiles CASCADE;
DROP TABLE IF EXISTS user_travel_preferences CASCADE;
DROP TABLE IF EXISTS user_profiles CASCADE;
DROP FUNCTION IF EXISTS update_updated_at_column CASCADE;