-- +goose Up
CREATE TABLE /* TEMPLATE: schema */incoming_spam (
  grantee_id BIGINT NOT NULL REFERENCES /* TEMPLATE: schema */users(user_id) ON DELETE CASCADE,
  file_id UUID NOT NULL REFERENCES /* TEMPLATE: schema */files(id) ON DELETE CASCADE,
  reported_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (grantee_id, file_id)
);
CREATE INDEX incoming_spam_expiry_idx ON /* TEMPLATE: schema */incoming_spam (reported_at);

-- +goose Down
DROP TABLE /* TEMPLATE: schema */incoming_spam;
