CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    username VARCHAR(64) UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

INSERT INTO users (username, password_hash)
VALUES ('demo', '$2a$10$9gOqciv/YbYmLs/AWmtEvOuQ9pnty9MOzU9Zz/VOQxbdSm9.hqj.i')
ON CONFLICT (username) DO NOTHING;
