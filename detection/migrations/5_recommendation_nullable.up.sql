-- If migration 4 already dropped the column, this is a no-op.
-- If the column still exists as NOT NULL, make it optional so INSERTs that
-- omit it succeed. The column is no longer read or written by the service.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'detection_flags' AND column_name = 'recommendation'
    ) THEN
        ALTER TABLE detection_flags
            ALTER COLUMN recommendation DROP NOT NULL,
            ALTER COLUMN recommendation SET DEFAULT '';
    END IF;
END $$;
