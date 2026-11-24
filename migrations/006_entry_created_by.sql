ALTER TABLE entries ADD COLUMN created_by INTEGER REFERENCES users(id);
UPDATE entries SET created_by = user_id WHERE created_by IS NULL;