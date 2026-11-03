-- +goose Up

-- member_id NULL = barang milik toko, sama seperti di products.
ALTER TABLE goods_receipts ALTER COLUMN member_id DROP NOT NULL;

-- physical_qty NULL = belum dihitung; baris itu tidak di-adjust saat posting.
ALTER TABLE stock_opname_items ALTER COLUMN physical_qty DROP NOT NULL;
