-- Exact integer columns for royalty amounts and controlled share.
-- Used by the detection engine (rules.Evaluate) via math/big.Rat.
-- The _cents columns store amounts normalised to 2 implied decimal places:
-- GrossCents=372000 represents 3720.00 SEK.
-- The float64 columns (gross_amount, net_amount, controlled_share) are retained
-- for the API response layer only and must not be used in detection calculations.
ALTER TABLE statement_lines
    ADD COLUMN gross_cents            BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN net_cents              BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN controlled_numerator   BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN controlled_denominator BIGINT NOT NULL DEFAULT 1;
