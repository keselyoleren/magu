-- +goose Up

-- Barcode buatan toko sekarang berupa kode pemilik + nomor urut per pemilik, dihitung dari produk yang ada.
DROP SEQUENCE internal_barcode_seq;
