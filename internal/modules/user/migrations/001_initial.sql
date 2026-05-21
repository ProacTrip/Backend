-- +migrate Up
-- =============================================================================
-- FUNCIONES Y UTILIDADES (sin lógica de negocio)
-- =============================================================================

-- Shared utility function (each module defines its own for self-containment)
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
-- =============================================================================
CREATE TABLE IF NOT EXISTS user_profiles (
    id UUID PRIMARY KEY DEFAULT uuidv7 (),
    user_id UUID NOT NULL UNIQUE,
    first_name VARCHAR(100),
    last_name VARCHAR(100),
    date_of_birth DATE,
    gender VARCHAR(20),
    nationality VARCHAR(2),
    phone VARCHAR(50),
    avatar_url TEXT,
    bio TEXT,
    language_code VARCHAR(5) NOT NULL DEFAULT 'es',
    currency_code VARCHAR(3) NOT NULL DEFAULT 'EUR',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_gender CHECK (
        gender IN (
            'male',
            'female',
            'non_binary',
            'prefer_not_to_say'
        )
    )
);

CREATE TRIGGER trg_profiles_updated_at
    BEFORE UPDATE ON user_profiles
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE INDEX idx_profiles_user_id ON user_profiles (user_id);

-- =============================================================================
-- USER TRAVEL PREFERENCES (1:1 with profile)
-- =============================================================================
CREATE TABLE IF NOT EXISTS user_travel_preferences (
    id UUID PRIMARY KEY DEFAULT uuidv7 (),
    user_id UUID NOT NULL UNIQUE REFERENCES user_profiles (user_id) ON DELETE CASCADE,
    preferred_class VARCHAR(20) NOT NULL DEFAULT 'economy',
    seat_preference VARCHAR(20),
    meal_preference VARCHAR(50),
    special_assistance TEXT [],
    preferred_airlines UUID [],
    preferred_hotels TEXT [],
    avoid_layovers BOOLEAN NOT NULL DEFAULT FALSE,
    max_layover_duration INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_preferred_class CHECK (
        preferred_class IN (
            'economy',
            'premium_economy',
            'business',
            'first'
        )
    ),
    CONSTRAINT chk_seat_preference CHECK (
        seat_preference IN (
            'window',
            'aisle',
            'middle',
            'no_preference'
        )
        OR seat_preference IS NULL
    )
);

CREATE TRIGGER trg_travel_prefs_updated_at
    BEFORE UPDATE ON user_travel_preferences
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- =============================================================================
-- USER MEDICAL PROFILES (1:1 with profile)
-- Fields with _enc suffix MUST be encrypted (AES-256-GCM) before persisting.
-- =============================================================================
CREATE TABLE IF NOT EXISTS user_medical_profiles (
    id UUID PRIMARY KEY DEFAULT uuidv7 (),
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
    id UUID PRIMARY KEY DEFAULT uuidv7 (),
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

INSERT INTO
    document_types (
        code,
        name,
        description,
        is_identity,
        requires_ocr,
        is_active,
        sort_order
    )
VALUES (
        'passport',
        'Passport',
        'International passport',
        true,
        true,
        true,
        1
    ),
    (
        'national_id',
        'National ID',
        'Government-issued identity card',
        true,
        true,
        true,
        2
    ),
    (
        'drivers_license',
        'Drivers License',
        'Driving license document',
        true,
        true,
        true,
        3
    ),
    (
        'visa',
        'Visa',
        'Travel visa document',
        false,
        true,
        true,
        4
    ),
    (
        'travel_insurance',
        'Travel Insurance',
        'Insurance policy document',
        false,
        false,
        true,
        5
    ),
    (
        'vaccination_cert',
        'Vaccination Certificate',
        'Health vaccination record',
        false,
        true,
        true,
        6
    ),
    (
        'boarding_pass',
        'Boarding Pass',
        'Flight boarding pass',
        false,
        false,
        true,
        7
    ),
    (
        'receipt',
        'Receipt',
        'Payment receipt',
        false,
        false,
        true,
        8
    )
ON CONFLICT (code) DO NOTHING;

-- =============================================================================
-- USER DOCUMENTS
-- =============================================================================
CREATE TABLE IF NOT EXISTS user_documents (
    id                UUID        PRIMARY KEY DEFAULT uuidv7(),
    user_id           UUID        NOT NULL REFERENCES user_profiles(user_id) ON DELETE CASCADE,
    document_type_id  UUID        NOT NULL REFERENCES document_types(id),
    file_name         VARCHAR(255) NOT NULL,
    file_size         INTEGER,
    mime_type         VARCHAR(100),
    storage_key       TEXT        NOT NULL,

-- Pipeline V3: async validation fields
detected_mime_type VARCHAR(100), -- Real MIME detected by server (magic numbers)
detected_size_bytes BIGINT, -- Real verified size
document_type VARCHAR(50), -- Detected category (passport, prescription, etc.)
failure_reason TEXT, -- Why processing failed, if applicable
verification_status VARCHAR(20) NOT NULL DEFAULT 'unverified' 
    CHECK (verification_status IN ('unverified', 'verified', 'rejected', 'manual_review', 'suspicious')),
ocr_status VARCHAR(20) NOT NULL DEFAULT 'queued',
ocr_data JSONB,
ocr_confidence DOUBLE PRECISION,
extracted_data JSONB,

-- Medical profile integration

has_newer_medical_data BOOLEAN NOT NULL DEFAULT FALSE, -- Newer data than current medical profile
    medical_update_summary JSONB,                          -- Summary of what would change in the medical profile

    valid_from        TIMESTAMPTZ,
    valid_until       TIMESTAMPTZ,
    document_number   VARCHAR(100),
    issuing_country   VARCHAR(2),
    metadata          JSONB,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT chk_ocr_status CHECK (
        ocr_status IN (
            'queued', 'processing', 'validating', 'sanitizing', 'ocr_processing',
            'completed', 'rejected', 'failed'
        )
    )
);

CREATE TRIGGER trg_user_documents_updated_at
    BEFORE UPDATE ON user_documents
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE INDEX idx_user_documents_user_id ON user_documents (user_id);

CREATE INDEX idx_user_documents_type_id ON user_documents (document_type_id);

CREATE INDEX idx_user_documents_ocr_status ON user_documents (ocr_status);

-- =============================================================================
-- MEDICAL PENDING UPDATES
-- Conflicts detected when OCR or NLP extract data that differs from the
-- current medical profile. The user reviews and accepts/rejects/customizes.
-- NOTE: Placed after user_documents to satisfy FK dependency.
-- =============================================================================
CREATE TABLE IF NOT EXISTS medical_pending_updates (
    id UUID PRIMARY KEY DEFAULT uuidv7 (),
    user_id UUID NOT NULL REFERENCES user_profiles (user_id) ON DELETE CASCADE,
    source_type VARCHAR(10) NOT NULL, -- 'ocr' | 'nlp'
    source_document_id UUID REFERENCES user_documents (id) ON DELETE CASCADE,
    conversation_id UUID,
    field_name VARCHAR(50) NOT NULL,
    current_value TEXT,
    proposed_value TEXT NOT NULL,
    suggested_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMPTZ NOT NULL DEFAULT (
        CURRENT_TIMESTAMP + interval '30 days'
    ),
    status VARCHAR(10) NOT NULL DEFAULT 'pending' CHECK (
        status IN (
            'pending',
            'accepted',
            'rejected'
        )
    ),
    resolved_at TIMESTAMPTZ,
    CONSTRAINT chk_pending_source CHECK (
        (
            source_type = 'ocr'
            AND source_document_id IS NOT NULL
        )
        OR (
            source_type = 'nlp'
            AND conversation_id IS NOT NULL
        )
    )
);

CREATE INDEX idx_medical_pending_user_status ON medical_pending_updates (user_id, status);

CREATE INDEX idx_medical_pending_expires ON medical_pending_updates (expires_at)
WHERE
    status = 'pending';

-- +migrate Down
-- Eliminar todas las tablas en orden inverso (para respetar FKs)
DROP TABLE IF EXISTS medical_pending_updates CASCADE;

DROP TABLE IF EXISTS user_documents CASCADE;

DROP TABLE IF EXISTS document_types CASCADE;

DROP TABLE IF EXISTS user_medical_profiles CASCADE;

DROP TABLE IF EXISTS user_travel_preferences CASCADE;

DROP TABLE IF EXISTS user_profiles CASCADE;

DROP FUNCTION IF EXISTS update_updated_at_column CASCADE;