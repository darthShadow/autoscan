-- Convert time column from DATETIME text to INTEGER unix timestamp

-- Create new table with INTEGER time column
CREATE TABLE scan_new (
    "folder" TEXT NOT NULL,
    "priority" INTEGER NOT NULL,
    "time" INTEGER NOT NULL,
    "relative_path" TEXT NOT NULL DEFAULT '',
    PRIMARY KEY(folder)
);

-- Copy data, setting all timestamps to one day ago
INSERT INTO scan_new (folder, priority, time, relative_path)
SELECT 
    folder, 
    priority, 
    STRFTIME('%s', 'now', '-1 day') as time,
    relative_path
FROM scan;

-- Drop old table
DROP TABLE scan;

-- Rename new table to original name
ALTER TABLE scan_new RENAME TO scan;
