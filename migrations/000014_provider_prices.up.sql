ALTER TABLE wb_provider_configs ADD COLUMN currency text NOT NULL DEFAULT '';
ALTER TABLE wb_provider_configs ADD COLUMN input_price_per_million double precision;
ALTER TABLE wb_provider_configs ADD COLUMN output_price_per_million double precision;
ALTER TABLE wb_provider_configs ADD CONSTRAINT wb_provider_prices_nonnegative CHECK ((input_price_per_million IS NULL OR input_price_per_million >= 0 AND input_price_per_million < 'Infinity'::float8) AND (output_price_per_million IS NULL OR output_price_per_million >= 0 AND output_price_per_million < 'Infinity'::float8));
