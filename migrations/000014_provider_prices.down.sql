ALTER TABLE wb_provider_configs DROP CONSTRAINT wb_provider_prices_nonnegative;
ALTER TABLE wb_provider_configs DROP COLUMN output_price_per_million;
ALTER TABLE wb_provider_configs DROP COLUMN input_price_per_million;
ALTER TABLE wb_provider_configs DROP COLUMN currency;
