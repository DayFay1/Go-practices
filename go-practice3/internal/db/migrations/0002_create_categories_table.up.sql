PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS categories (
  id      INTEGER PRIMARY KEY AUTOINCREMENT,
  name    TEXT    NOT NULL,
  user_id INTEGER,
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_categories_user_id ON categories(user_id);

CREATE UNIQUE INDEX IF NOT EXISTS uq_categories_user_name
  ON categories(user_id, name)
  WHERE user_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_categories_global_name
  ON categories(name)
  WHERE user_id IS NULL;
