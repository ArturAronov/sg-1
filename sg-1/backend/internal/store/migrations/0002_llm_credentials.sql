CREATE TABLE llm_credentials (
    user_id uuid NOT NULL REFERENCES users(id),
    provider text NOT NULL CHECK (provider IN ('openai', 'anthropic', 'glm', 'kimi')),
    key_ciphertext bytea NOT NULL CHECK (octet_length(key_ciphertext) >= 45),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, provider)
);

-- Keys are encrypted by the application before reaching SQL. No plaintext,
-- suffixes, or credentials are written into audit metadata.
