CREATE TABLE requests (
    id uuid PRIMARY KEY,
    title text NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 200),
    status text NOT NULL CHECK (status IN ('created', 'processed')),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE INDEX requests_status_idx ON requests (status);
CREATE INDEX requests_created_at_idx ON requests (created_at);
