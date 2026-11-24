ALTER TABLE books ADD COLUMN slug TEXT DEFAULT '';
UPDATE books SET slug = lower(substr('00000000' || hex(abs(random())), -8, 8)) WHERE slug = '';