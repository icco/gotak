
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    provider VARCHAR(32) NOT NULL,
    provider_id VARCHAR(128) NOT NULL,
    email VARCHAR(255),
    name VARCHAR(128),
    avatar_url VARCHAR(512),
    password_hash VARCHAR(255), -- nullable for social login
    preferences JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(provider, provider_id),
    UNIQUE(email)
);

DROP TABLE IF EXISTS users;
