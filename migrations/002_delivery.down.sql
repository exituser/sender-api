-- These columns are part of the canonical schema in 001_initial.up.sql.
-- Keep them on rollback: removing them would destroy existing message and
-- domain-verification data from the base migration.
SELECT 1;
