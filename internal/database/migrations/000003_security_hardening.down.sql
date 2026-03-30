-- Reverse of 000003: drop security tables and indexes.

DROP INDEX IF EXISTS idx_audit_logs_action;
DROP INDEX IF EXISTS idx_audit_logs_resource;
DROP INDEX IF EXISTS idx_audit_logs_actor;
DROP INDEX IF EXISTS idx_audit_logs_org;
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS login_attempts;
