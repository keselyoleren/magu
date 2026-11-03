-- +goose Up

CREATE TABLE vendor_returns (
  id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  member_id   BIGINT NOT NULL REFERENCES members (id),
  returned_by BIGINT NOT NULL REFERENCES users (id),
  note        TEXT,
  returned_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX vendor_returns_member_id_idx ON vendor_returns (member_id, returned_at DESC);

CREATE TABLE vendor_return_items (
  id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  vendor_return_id BIGINT NOT NULL REFERENCES vendor_returns (id) ON DELETE CASCADE,
  product_id       BIGINT NOT NULL REFERENCES products (id),
  qty              INT NOT NULL CHECK (qty > 0),
  UNIQUE (vendor_return_id, product_id)
);

-- Satu shift = satu kasir dari buka sampai tutup laci. expected_cash dan counted_cash diisi saat tutup.
CREATE TABLE cash_shifts (
  id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  cashier_id    BIGINT NOT NULL REFERENCES users (id),
  opened_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  opening_cash  BIGINT NOT NULL CHECK (opening_cash >= 0),
  closed_at     TIMESTAMPTZ,
  expected_cash BIGINT,
  counted_cash  BIGINT CHECK (counted_cash >= 0),
  note          TEXT
);

CREATE UNIQUE INDEX cash_shifts_one_open_idx ON cash_shifts (cashier_id) WHERE closed_at IS NULL;
CREATE INDEX cash_shifts_opened_at_idx ON cash_shifts (opened_at DESC);

-- Nomor urut untuk barcode buatan toko (EAN-13 berawalan 29, rentang yang dicadangkan untuk pemakaian internal).
CREATE SEQUENCE internal_barcode_seq;
