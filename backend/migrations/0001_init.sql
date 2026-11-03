-- +goose Up

CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- +goose StatementBegin
CREATE FUNCTION set_updated_at() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TABLE users (
  id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  username      TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  full_name     TEXT NOT NULL,
  role          TEXT NOT NULL CHECK (role IN ('admin', 'cashier')),
  is_active     BOOLEAN NOT NULL DEFAULT TRUE,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE members (
  id                  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  code                TEXT NOT NULL UNIQUE,
  name                TEXT NOT NULL,
  phone               TEXT,
  address             TEXT,
  bank_name           TEXT,
  bank_account_number TEXT,
  bank_account_name   TEXT,
  commission_bps      INT NOT NULL CHECK (commission_bps BETWEEN 0 AND 10000),
  is_active           BOOLEAN NOT NULL DEFAULT TRUE,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TRIGGER members_set_updated_at BEFORE UPDATE ON members
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- member_id NULL = barang milik toko (bukan titipan).
-- commission_bps NULL = ikut komisi member.
CREATE TABLE products (
  id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  member_id      BIGINT REFERENCES members (id),
  barcode        TEXT UNIQUE,
  name           TEXT NOT NULL,
  price          BIGINT NOT NULL CHECK (price >= 0),
  commission_bps INT CHECK (commission_bps BETWEEN 0 AND 10000),
  stock          INT NOT NULL DEFAULT 0,
  attributes     JSONB NOT NULL DEFAULT '{}',
  is_active      BOOLEAN NOT NULL DEFAULT TRUE,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (member_id IS NOT NULL OR commission_bps IS NULL)
);

CREATE INDEX products_member_id_idx ON products (member_id);
CREATE INDEX products_name_trgm_idx ON products USING gin (name gin_trgm_ops);
CREATE INDEX products_updated_at_idx ON products (updated_at, id);

-- Perubahan stok sengaja tidak menyentuh updated_at supaya delta-sync katalog POS tidak ramai.
CREATE TRIGGER products_set_updated_at
  BEFORE UPDATE OF member_id, barcode, name, price, commission_bps, attributes, is_active ON products
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE goods_receipts (
  id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  member_id   BIGINT NOT NULL REFERENCES members (id),
  received_by BIGINT NOT NULL REFERENCES users (id),
  note        TEXT,
  received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX goods_receipts_member_id_idx ON goods_receipts (member_id, received_at DESC);

CREATE TABLE goods_receipt_items (
  id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  goods_receipt_id BIGINT NOT NULL REFERENCES goods_receipts (id) ON DELETE CASCADE,
  product_id       BIGINT NOT NULL REFERENCES products (id),
  qty              INT NOT NULL CHECK (qty > 0),
  UNIQUE (goods_receipt_id, product_id)
);

CREATE TABLE stock_opnames (
  id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  member_id  BIGINT REFERENCES members (id),
  status     TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'posted')),
  note       TEXT,
  created_by BIGINT NOT NULL REFERENCES users (id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  posted_at  TIMESTAMPTZ
);

CREATE TABLE stock_opname_items (
  id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  stock_opname_id BIGINT NOT NULL REFERENCES stock_opnames (id) ON DELETE CASCADE,
  product_id      BIGINT NOT NULL REFERENCES products (id),
  system_qty      INT NOT NULL,
  physical_qty    INT NOT NULL CHECK (physical_qty >= 0),
  diff            INT GENERATED ALWAYS AS (physical_qty - system_qty) STORED,
  UNIQUE (stock_opname_id, product_id)
);

-- Buku besar stok: setiap perubahan products.stock wajib punya satu baris di sini.
-- ref_id menunjuk dokumen sumber sesuai reason (goods_receipts, transactions, stock_opnames).
CREATE TABLE stock_movements (
  id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  product_id BIGINT NOT NULL REFERENCES products (id),
  qty_change INT NOT NULL CHECK (qty_change <> 0),
  reason     TEXT NOT NULL CHECK (reason IN ('receipt', 'sale', 'void', 'opname', 'vendor_return')),
  ref_id     BIGINT,
  user_id    BIGINT REFERENCES users (id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX stock_movements_product_id_idx ON stock_movements (product_id, created_at DESC);

CREATE TABLE vendor_settlements (
  id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  member_id        BIGINT NOT NULL REFERENCES members (id),
  period_start     DATE NOT NULL,
  period_end       DATE NOT NULL,
  gross_sales      BIGINT NOT NULL DEFAULT 0,
  commission_total BIGINT NOT NULL DEFAULT 0,
  payout_amount    BIGINT NOT NULL DEFAULT 0,
  status           TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'paid')),
  note             TEXT,
  created_by       BIGINT NOT NULL REFERENCES users (id),
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  paid_at          TIMESTAMPTZ,
  CHECK (period_end >= period_start),
  CHECK (gross_sales = commission_total + payout_amount)
);

CREATE INDEX vendor_settlements_member_id_idx ON vendor_settlements (member_id, period_end DESC);

CREATE SEQUENCE invoice_seq;

CREATE TABLE transactions (
  id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  invoice_no TEXT NOT NULL UNIQUE
             DEFAULT 'INV' || to_char(now(), 'YYMMDD') || lpad(nextval('invoice_seq')::text, 6, '0'),
  client_ref UUID NOT NULL UNIQUE,
  cashier_id BIGINT NOT NULL REFERENCES users (id),
  status     TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'paid', 'void')),
  total      BIGINT NOT NULL CHECK (total >= 0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  paid_at    TIMESTAMPTZ,
  voided_at  TIMESTAMPTZ
);

CREATE INDEX transactions_created_at_idx ON transactions (created_at DESC);
CREATE INDEX transactions_paid_at_idx ON transactions (paid_at) WHERE status = 'paid';

-- Harga dan komisi di-snapshot per baris: perubahan master data tidak mengubah transaksi lama.
-- commission_amount = hak toko, vendor_amount = hak penitip.
CREATE TABLE transaction_items (
  id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  transaction_id    BIGINT NOT NULL REFERENCES transactions (id) ON DELETE CASCADE,
  product_id        BIGINT NOT NULL REFERENCES products (id),
  member_id         BIGINT REFERENCES members (id),
  product_name      TEXT NOT NULL,
  unit_price        BIGINT NOT NULL CHECK (unit_price >= 0),
  qty               INT NOT NULL CHECK (qty > 0),
  line_total        BIGINT NOT NULL,
  commission_bps    INT NOT NULL CHECK (commission_bps BETWEEN 0 AND 10000),
  commission_amount BIGINT NOT NULL CHECK (commission_amount >= 0),
  vendor_amount     BIGINT NOT NULL CHECK (vendor_amount >= 0),
  settlement_id     BIGINT REFERENCES vendor_settlements (id),
  CHECK (line_total = unit_price * qty),
  CHECK (commission_amount + vendor_amount = line_total)
);

CREATE INDEX transaction_items_transaction_id_idx ON transaction_items (transaction_id);
CREATE INDEX transaction_items_settlement_id_idx ON transaction_items (settlement_id)
  WHERE settlement_id IS NOT NULL;
CREATE INDEX transaction_items_unsettled_idx ON transaction_items (member_id)
  WHERE settlement_id IS NULL AND member_id IS NOT NULL;

-- Split payment = beberapa baris payments untuk satu transaksi.
CREATE TABLE payments (
  id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  transaction_id  BIGINT NOT NULL REFERENCES transactions (id) ON DELETE CASCADE,
  method          TEXT NOT NULL CHECK (method IN ('cash', 'debit', 'qris')),
  amount          BIGINT NOT NULL CHECK (amount > 0),
  tendered        BIGINT CHECK (tendered >= amount),
  status          TEXT NOT NULL CHECK (status IN ('pending', 'paid', 'failed', 'expired')),
  provider_ref    TEXT UNIQUE,
  reference       TEXT,
  gateway_payload JSONB,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  paid_at         TIMESTAMPTZ
);

CREATE INDEX payments_transaction_id_idx ON payments (transaction_id);
