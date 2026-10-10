CREATE INDEX requests_status_created_at_id_idx
    ON requests (status, created_at DESC, id DESC);
