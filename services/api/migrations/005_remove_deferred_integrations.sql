-- Removes tables from the briefly introduced future-provider prototype.
-- Safe on installations where that prototype was never migrated.
DROP TABLE IF EXISTS payment_orders;
DROP TABLE IF EXISTS billing_events;
DROP TABLE IF EXISTS jobs;
DROP TABLE IF EXISTS credit_ledger;
DROP TABLE IF EXISTS credit_accounts;
