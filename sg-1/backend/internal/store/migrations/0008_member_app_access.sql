ALTER TABLE project_access_settings DROP CONSTRAINT project_access_settings_mode_check;
ALTER TABLE project_access_settings ADD CONSTRAINT project_access_settings_mode_check CHECK (mode IN ('members', 'password', 'public'));
ALTER TABLE project_access_settings ALTER COLUMN mode SET DEFAULT 'members';
-- Existing public/password projects retain their explicitly selected mode.
