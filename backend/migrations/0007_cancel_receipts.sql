-- +goose Up

-- cancelled_at NULL = penerimaan masih berlaku.
ALTER TABLE goods_receipts
  ADD COLUMN cancelled_at TIMESTAMPTZ,
  ADD COLUMN cancelled_by BIGINT REFERENCES users (id);
