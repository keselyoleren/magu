package consignment

import (
	"context"
	"errors"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"magu/internal/drive"
	"magu/internal/pgerr"
)

var (
	ErrMemberNotFound  = errors.New("member tidak ditemukan")
	ErrMemberCodeTaken = errors.New("kode member sudah dipakai")
)

type Service struct {
	db      *pgxpool.Pool
	drive   *drive.Client
	driveMu sync.Mutex
}

// drive boleh nil: laporan tetap bisa diunduh, hanya pengiriman ke Google Drive yang mati.
func NewService(db *pgxpool.Pool, drive *drive.Client) *Service {
	return &Service{db: db, drive: drive}
}

type Member struct {
	ID                int64  `json:"id"`
	Code              string `json:"code"`
	Name              string `json:"name"`
	Phone             string `json:"phone"`
	Email             string `json:"email"`
	Address           string `json:"address"`
	BankName          string `json:"bank_name"`
	BankAccountNumber string `json:"bank_account_number"`
	BankAccountName   string `json:"bank_account_name"`
	CommissionBps     int    `json:"commission_bps"`
	IsActive          bool   `json:"is_active"`
	DriveFolderID     string `json:"drive_folder_id"`
}

func (s *Service) ListMembers(ctx context.Context) ([]Member, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, code, name, COALESCE(phone, ''), COALESCE(email, ''), COALESCE(address, ''), COALESCE(bank_name, ''),
		       COALESCE(bank_account_number, ''), COALESCE(bank_account_name, ''), commission_bps, is_active,
		       COALESCE(drive_folder_id, '')
		FROM members ORDER BY name`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Member])
}

func (s *Service) CreateMember(ctx context.Context, m *Member) error {
	err := s.db.QueryRow(ctx, `
		INSERT INTO members (code, name, phone, address, bank_name, bank_account_number, bank_account_name,
		                     commission_bps, is_active, email)
		VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), $8, $9, NULLIF($10, ''))
		RETURNING id`,
		m.Code, m.Name, m.Phone, m.Address, m.BankName, m.BankAccountNumber, m.BankAccountName,
		m.CommissionBps, m.IsActive, m.Email).Scan(&m.ID)
	return memberError(err)
}

func (s *Service) UpdateMember(ctx context.Context, m *Member) error {
	err := s.db.QueryRow(ctx, `
		UPDATE members
		SET code = $2, name = $3, phone = NULLIF($4, ''), address = NULLIF($5, ''), bank_name = NULLIF($6, ''),
		    bank_account_number = NULLIF($7, ''), bank_account_name = NULLIF($8, ''),
		    commission_bps = $9, is_active = $10, email = NULLIF($11, '')
		WHERE id = $1
		RETURNING id`,
		m.ID, m.Code, m.Name, m.Phone, m.Address, m.BankName, m.BankAccountNumber, m.BankAccountName,
		m.CommissionBps, m.IsActive, m.Email).Scan(&m.ID)
	return memberError(err)
}

func memberError(err error) error {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrMemberNotFound
	case pgerr.IsUnique(err):
		return ErrMemberCodeTaken
	case pgerr.IsForeignKey(err):
		return ErrMemberNotFound
	}
	return err
}
