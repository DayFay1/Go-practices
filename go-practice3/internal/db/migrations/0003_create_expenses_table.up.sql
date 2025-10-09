PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS expenses (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id     INTEGER NOT NULL,
  category_id INTEGER NOT NULL,
  amount      NUMERIC NOT NULL CHECK (amount > 0),
  currency    TEXT    NOT NULL,               
  spent_at    DATETIME NOT NULL,              
  created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  note        TEXT,
  FOREIGN KEY (user_id)     REFERENCES users(id)      ON DELETE CASCADE,
  FOREIGN KEY (category_id) REFERENCES categories(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_expenses_user_id         ON expenses(user_id);
CREATE INDEX IF NOT EXISTS idx_expenses_user_spent_at   ON expenses(user_id, spent_at);
