CREATE TABLE users (
  user_id      INTEGER PRIMARY KEY,      -- Telegram user ID
  created_at   TEXT NOT NULL
);

CREATE TABLE categories (
  id           INTEGER PRIMARY KEY,
  user_id      INTEGER NOT NULL REFERENCES users(user_id),
  name         TEXT    NOT NULL,
  kind         TEXT    NOT NULL CHECK (kind IN ('expense','income')),
  active       INTEGER NOT NULL DEFAULT 1,
  sort_order   INTEGER NOT NULL DEFAULT 0,
  created_at   TEXT    NOT NULL,
  UNIQUE (user_id, kind, name COLLATE NOCASE)
);

CREATE TABLE transactions (
  id           INTEGER PRIMARY KEY,
  user_id      INTEGER NOT NULL REFERENCES users(user_id),
  occurred_on  TEXT    NOT NULL,         -- 'YYYY-MM-DD', waktu lokal WIB
  kind         TEXT    NOT NULL CHECK (kind IN ('expense','income')),
  category_id  INTEGER NOT NULL REFERENCES categories(id),
  amount       INTEGER NOT NULL CHECK (amount > 0),
  note         TEXT    NOT NULL DEFAULT '',
  created_at   TEXT    NOT NULL,
  updated_at   TEXT    NOT NULL          -- diisi juga saat INSERT
);
CREATE INDEX idx_tx_user_date ON transactions(user_id, occurred_on);

CREATE TABLE periods (
  id             INTEGER PRIMARY KEY,
  user_id        INTEGER NOT NULL REFERENCES users(user_id),
  name           TEXT    NOT NULL,
  start_day      INTEGER NOT NULL CHECK (start_day BETWEEN 2 AND 28),
  end_day        INTEGER NOT NULL CHECK (end_day   BETWEEN 1 AND 27),
  effective_from TEXT    NOT NULL,       -- 'YYYY-MM-DD'
  created_at     TEXT    NOT NULL,
  CHECK (end_day = start_day - 1),
  UNIQUE (user_id, effective_from)
);

CREATE TABLE settings (
  user_id            INTEGER PRIMARY KEY REFERENCES users(user_id),
  reminder_enabled   INTEGER NOT NULL DEFAULT 1,
  reminder_time      TEXT    NOT NULL DEFAULT '19:00',
  last_reminder_date TEXT    NOT NULL DEFAULT '',  -- 'YYYY-MM-DD' WIB, '' = belum pernah
  persona            TEXT    NOT NULL DEFAULT 'penjaga'
                             CHECK (persona IN ('netral','penjaga','posesif','softboy'))
);

CREATE TABLE conversations (
  user_id    INTEGER PRIMARY KEY REFERENCES users(user_id),
  state      TEXT    NOT NULL,
  payload    TEXT    NOT NULL DEFAULT '{}',      -- JSON state sementara
  updated_at TEXT    NOT NULL
);
