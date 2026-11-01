ALTER TABLE drying_jobs
    ADD COLUMN IF NOT EXISTS bag_count integer NOT NULL DEFAULT 0 CHECK (bag_count >= 0);
