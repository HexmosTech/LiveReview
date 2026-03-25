package archive

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/livereview/pkg/models"
)

func DiffReviewLoadUser(db *sql.DB, userID int64) (models.User, error) {
	if userID <= 0 {
		return models.User{}, fmt.Errorf("invalid userID: %d", userID)
	}

	var user models.User
	err := db.QueryRow(`
		SELECT id, email, first_name, last_name
		FROM users
		WHERE id = $1
	`, userID).Scan(&user.ID, &user.Email, &user.FirstName, &user.LastName)
	if err != nil {
		return models.User{}, err
	}
	return user, nil
}

func DiffReviewCreateTempWorkspace() (string, error) {
	return os.MkdirTemp("", "lr-diff-review-")
}

func DiffReviewRemoveWorkspace(path string) error {
	return os.RemoveAll(path)
}

func DiffReviewWriteUploadedZip(path string, zipBytes []byte) error {
	return os.WriteFile(path, zipBytes, 0600)
}

func DiffReviewReadExtractedDiff(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func DiffReviewEnsureParentDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0700)
}

func DiffReviewOpenExtractedFile(path string, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
}
