-- Add Condition column to Device table
-- This stores the physical condition of the device (new, like-new, good, fair, poor)
ALTER TABLE Device 
ADD COLUMN IF NOT EXISTS Condition VARCHAR(20) DEFAULT 'good' 
CHECK (Condition IN ('new', 'like-new', 'good', 'fair', 'poor'));

-- Update existing records to have default condition
UPDATE Device 
SET Condition = 'good' 
WHERE Condition IS NULL;
