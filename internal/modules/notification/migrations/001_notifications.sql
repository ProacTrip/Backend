-- +migrate Up

-- Shared utility function
CREATE OR REPLACE FUNCTION update_updated_at_column()
    RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;


-- =============================================================================
-- NOTIFICATIONS
-- Records every notification dispatched through any channel.
-- template_code stores the Resend template identifier (e.g. 'flight-alert')
-- for audit and debugging purposes — no local template management.
-- delivered_at and opened_at are populated from Resend webhook events.
-- provider_message_id stores the Resend message ID for webhook correlation.
-- =============================================================================
CREATE TABLE IF NOT EXISTS notifications (
    id                  UUID         PRIMARY KEY DEFAULT uuidv7(),
    user_id             UUID         NOT NULL,
    template_code       VARCHAR(50),
    type                VARCHAR(20)  NOT NULL,
    channel             VARCHAR(20)  NOT NULL,
    subject             VARCHAR(255),
    content             TEXT         NOT NULL,
    data                JSONB        NOT NULL DEFAULT '{}',
    status              VARCHAR(20)  NOT NULL DEFAULT 'pending',
    sent_at             TIMESTAMPTZ,
    delivered_at        TIMESTAMPTZ,
    opened_at           TIMESTAMPTZ,
    error_message       TEXT,
    provider_message_id VARCHAR(255),
    metadata            JSONB        NOT NULL DEFAULT '{}',
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT chk_notif_status CHECK (
        status IN ('pending', 'processing', 'sent', 'delivered', 'failed', 'bounced')
    ),
    CONSTRAINT chk_notif_type CHECK (
        type IN ('transactional', 'marketing', 'system')
    ),
    CONSTRAINT chk_notif_channel CHECK (
        channel IN ('email', 'sms', 'websocket')
    )
);

CREATE TRIGGER trg_notifications_updated_at
    BEFORE UPDATE ON notifications
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE INDEX idx_notifications_user_id ON notifications(user_id, created_at DESC);
CREATE INDEX idx_notifications_status  ON notifications(status)
    WHERE status IN ('pending', 'processing');
-- Enables fast webhook correlation: Resend sends its message ID back on every event
CREATE INDEX idx_notifications_provider_msg ON notifications(provider_message_id)
    WHERE provider_message_id IS NOT NULL;


-- =============================================================================
-- NOTIFICATION READS
-- Tracks in-app read status for websocket notifications.
-- Intentionally separate from opened_at: opened_at is the Resend email-open
-- pixel event, while this table captures explicit in-app acknowledgement.
-- =============================================================================
CREATE TABLE IF NOT EXISTS notification_reads (
    id              UUID        PRIMARY KEY DEFAULT uuidv7(),
    user_id         UUID        NOT NULL,
    notification_id UUID        NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
    read_at         TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT uq_notification_read UNIQUE (user_id, notification_id)
);

CREATE INDEX idx_notif_reads_user_id         ON notification_reads(user_id);
CREATE INDEX idx_notif_reads_notification_id ON notification_reads(notification_id);