-- Migration 010: Add device specification columns (CPU, RAM, Storage)
ALTER TABLE Device
    ADD COLUMN IF NOT EXISTS CPU     VARCHAR(100),
    ADD COLUMN IF NOT EXISTS RAM     VARCHAR(50),
    ADD COLUMN IF NOT EXISTS Storage VARCHAR(100);
