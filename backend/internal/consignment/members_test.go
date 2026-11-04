package consignment

import (
	"context"
	"testing"
)

func TestDeleteMember(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	if err := f.svc.DeleteMember(ctx, f.memberID); err != ErrMemberInUse {
		t.Fatalf("member dengan produk: err = %v, mau ErrMemberInUse", err)
	}

	m := Member{Code: "M2", Name: "Salah input", CommissionBps: 1000, IsActive: true}
	if err := f.svc.CreateMember(ctx, &m); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.DeleteMember(ctx, m.ID); err != nil {
		t.Fatalf("member tanpa riwayat: %v", err)
	}
	if err := f.svc.DeleteMember(ctx, m.ID); err != ErrMemberNotFound {
		t.Fatalf("hapus kedua: err = %v, mau ErrMemberNotFound", err)
	}
}
