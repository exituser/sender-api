DROP INDEX IF EXISTS idx_teams_stripe_subscription_id;
DROP INDEX IF EXISTS idx_teams_stripe_customer_id;
ALTER TABLE teams
    DROP COLUMN IF EXISTS cancel_at_period_end,
    DROP COLUMN IF EXISTS current_period_end,
    DROP COLUMN IF EXISTS billing_status;
