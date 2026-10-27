-- +goose Up
ALTER TABLE engagement.notification_preferences ADD COLUMN IF NOT EXISTS phone_number TEXT;
CREATE INDEX IF NOT EXISTS idx_notification_preferences_phone_number ON engagement.notification_preferences (phone_number);

ALTER TABLE engagement.notification_jobs ADD COLUMN IF NOT EXISTS read_at TIMESTAMPTZ;
ALTER TABLE engagement.notification_jobs ADD COLUMN IF NOT EXISTS provider TEXT DEFAULT 'fonnte';

-- +goose Down
DROP INDEX IF EXISTS engagement.idx_notification_preferences_phone_number;
ALTER TABLE engagement.notification_preferences DROP COLUMN IF EXISTS phone_number;
ALTER TABLE engagement.notification_jobs DROP COLUMN IF EXISTS read_at;
ALTER TABLE engagement.notification_jobs DROP COLUMN IF EXISTS provider;
