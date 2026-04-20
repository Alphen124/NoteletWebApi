-- Migration 011: Add GPU column to Device table
ALTER TABLE Device
    ADD COLUMN IF NOT EXISTS GPU VARCHAR(100);