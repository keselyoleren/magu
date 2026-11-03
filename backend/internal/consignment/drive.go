package consignment

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

var ErrDriveDisabled = errors.New("Google Drive belum diatur di server")

const xlsxContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

func (s *Service) DriveEnabled() bool {
	return s.drive != nil
}

// SyncMemberFolder memastikan member punya folder, dan hanya email terbarunya yang bisa membukanya.
// Dikunci karena dua permintaan bersamaan untuk member baru akan membuat dua folder; backend hanya satu proses.
func (s *Service) SyncMemberFolder(ctx context.Context, memberID int64) (string, error) {
	if s.drive == nil {
		return "", ErrDriveDisabled
	}
	s.driveMu.Lock()
	defer s.driveMu.Unlock()

	var code, name, email, folderID, permissionID, sharedEmail string
	err := s.db.QueryRow(ctx, `
		SELECT code, name, COALESCE(email, ''), COALESCE(drive_folder_id, ''),
		       COALESCE(drive_permission_id, ''), COALESCE(drive_shared_email, '')
		FROM members WHERE id = $1`, memberID).
		Scan(&code, &name, &email, &folderID, &permissionID, &sharedEmail)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrMemberNotFound
	}
	if err != nil {
		return "", err
	}

	if folderID == "" {
		if folderID, err = s.drive.CreateFolder(ctx, code+" - "+name); err != nil {
			return "", err
		}
		_, err = s.db.Exec(ctx, `UPDATE members SET drive_folder_id = $2 WHERE id = $1`, memberID, folderID)
		if err != nil {
			return "", err
		}
	}

	if email == sharedEmail {
		return folderID, nil
	}
	if permissionID != "" {
		if err := s.drive.Unshare(ctx, folderID, permissionID); err != nil {
			return "", err
		}
	}
	permissionID = ""
	if email != "" {
		if permissionID, err = s.drive.Share(ctx, folderID, email); err != nil {
			return "", err
		}
	}
	_, err = s.db.Exec(ctx, `
		UPDATE members SET drive_permission_id = NULLIF($2, ''), drive_shared_email = NULLIF($3, '')
		WHERE id = $1`, memberID, permissionID, email)
	return folderID, err
}

func (s *Service) SendToMemberFolder(ctx context.Context, memberID int64, filename string, data []byte) (string, error) {
	folderID, err := s.SyncMemberFolder(ctx, memberID)
	if err != nil {
		return "", err
	}
	return folderID, s.drive.Upload(ctx, folderID, filename, xlsxContentType, data)
}

func (s *Service) MarkSettlementSent(ctx context.Context, id int64) error {
	_, err := s.db.Exec(ctx, `UPDATE vendor_settlements SET drive_sent_at = now() WHERE id = $1`, id)
	return err
}
