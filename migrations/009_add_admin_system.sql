-- Migration 009: Add admin system
-- Adds is_admin flag to appuser and is_admin_device flag to Device

-- Add is_admin column to appuser
ALTER TABLE appuser
  ADD COLUMN IF NOT EXISTS is_admin BOOLEAN NOT NULL DEFAULT FALSE;

-- Add is_admin_device column to Device
ALTER TABLE Device
  ADD COLUMN IF NOT EXISTS is_admin_device BOOLEAN NOT NULL DEFAULT FALSE;
