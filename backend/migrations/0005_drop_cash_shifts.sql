-- +goose Up

-- Fitur shift kasir dihapus; penggantinya rekap harian yang dihitung langsung dari transaksi.
DROP TABLE cash_shifts;
