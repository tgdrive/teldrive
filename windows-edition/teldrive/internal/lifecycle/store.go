// Package lifecycle implements recoverable trash and manually marked spam.
package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const Retention = 30 * 24 * time.Hour

var ErrInvalid = errors.New("solicitud no válida")
var ErrNotFound = errors.New("no se encontró el archivo en esta carpeta")

type Item struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Type          string     `json:"type"`
	Size          *int64     `json:"size"`
	MimeType      string     `json:"mimeType"`
	ParentID      *string    `json:"parentId" gorm:"column:parent_id"`
	LifecycleRoot *string    `json:"-"`
	LifecycleAt   *time.Time `json:"movedAt"`
	UpdatedAt     *time.Time `json:"updatedAt"`
}

func List(ctx context.Context, db *gorm.DB, user int64, state string) ([]Item, error) {
	if state != "trash" && state != "spam" {
		return nil, ErrInvalid
	}
	items := []Item{}
	err := db.WithContext(ctx).Table("teldrive.files").Select("id,name,type,size,mime_type,parent_id,lifecycle_at,updated_at").Where("user_id = ? AND status = ? AND id = lifecycle_root", user, state).Order("lifecycle_at DESC").Find(&items).Error
	return items, err
}

func Change(ctx context.Context, db *gorm.DB, user int64, ids []string, action, state string) error {
	if len(ids) == 0 || len(ids) > 500 {
		return ErrInvalid
	}
	if action != "trash" && action != "spam" && action != "restore" && action != "delete" {
		return ErrInvalid
	}
	if (action == "restore" || action == "delete") && state != "trash" && state != "spam" {
		return ErrInvalid
	}
	for _, id := range ids {
		if _, err := uuid.Parse(id); err != nil {
			return ErrInvalid
		}
	}
	uniqueIDs := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		if !seen[id] {
			uniqueIDs = append(uniqueIDs, id)
			seen[id] = true
		}
	}
	ids = uniqueIDs
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Serialize lifecycle operations for this account, including expiration.
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", user).Error; err != nil {
			return err
		}
		if action == "trash" || action == "spam" {
			var count int64
			if err := tx.Table("teldrive.files").Where("id IN ? AND user_id = ? AND status = 'active'", ids, user).Count(&count).Error; err != nil {
				return err
			}
			if int(count) != len(ids) {
				return ErrNotFound
			}
			var roots []string
			query := `WITH RECURSIVE ancestors AS (
    SELECT id AS original,parent_id FROM teldrive.files WHERE id IN (?) AND user_id = ?
    UNION SELECT a.original,f.parent_id FROM teldrive.files f JOIN ancestors a ON f.id = a.parent_id WHERE f.user_id = ?
   ) SELECT DISTINCT original FROM ancestors WHERE original NOT IN (SELECT original FROM ancestors WHERE parent_id IN (?))`
			if err := tx.Raw(query, ids, user, user, ids).Scan(&roots).Error; err != nil {
				return err
			}
			ids = roots
		}
		for _, id := range ids {
			var item Item
			sourceState := "active"
			if action == "restore" || action == "delete" {
				sourceState = state
			}
			if err := tx.Table("teldrive.files").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ? AND status = ?", id, user, sourceState).First(&item).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrNotFound
				}
				return err
			}
			if action == "trash" || action == "spam" {
				if item.ParentID == nil {
					return ErrInvalid
				} // Protect the account root.
				query := `WITH RECURSIVE tree AS (
				 SELECT id FROM teldrive.files WHERE id = ? AND user_id = ?
				 UNION SELECT f.id FROM teldrive.files f JOIN tree t ON f.parent_id = t.id WHERE f.user_id = ? AND f.status != 'pending_deletion'
				) UPDATE teldrive.files SET status = ?, lifecycle_root = ?, lifecycle_at = NOW() WHERE id IN (SELECT id FROM tree) AND user_id = ?`
				if err := tx.Exec(query, id, user, user, action, id, user).Error; err != nil {
					return err
				}
			} else {
				if item.LifecycleRoot == nil || *item.LifecycleRoot != id {
					return ErrInvalid
				}
				if action == "delete" {
					if err := purgeGroup(tx, user, id); err != nil {
						return err
					}
					continue
				}
				var parentCount int64
				if err := tx.Table("teldrive.files").Where("id = ? AND user_id = ? AND status = 'active'", item.ParentID, user).Count(&parentCount).Error; err != nil {
					return err
				}
				if parentCount == 0 {
					var root Item
					if err := tx.Table("teldrive.files").Where("user_id = ? AND parent_id IS NULL AND type = 'folder' AND status = 'active'", user).First(&root).Error; err != nil {
						return err
					}
					item.ParentID = &root.ID
				}
				var conflict int64
				if err := tx.Table("teldrive.files").Where("user_id = ? AND parent_id = ? AND name = ? AND status = 'active'", user, item.ParentID, item.Name).Count(&conflict).Error; err != nil {
					return err
				}
				if conflict > 0 {
					item.Name = fmt.Sprintf("%s (restaurado %s)", item.Name, id[:8])
				}
				if err := tx.Table("teldrive.files").Where("id = ? AND user_id = ?", id, user).Updates(map[string]any{"name": item.Name, "parent_id": item.ParentID}).Error; err != nil {
					return err
				}
				if err := tx.Table("teldrive.files").Where("lifecycle_root = ? AND user_id = ? AND status = ?", id, user, state).Updates(map[string]any{"status": "active", "lifecycle_root": nil, "lifecycle_at": nil}).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func purgeGroup(tx *gorm.DB, user int64, root string) error {
	if err := tx.Exec("UPDATE teldrive.files SET status = 'pending_deletion', lifecycle_at = NULL WHERE lifecycle_root = ? AND user_id = ? AND type = 'file'", root, user).Error; err != nil {
		return err
	}
	// Folder metadata is removed; Telegram file parts use the existing cleanup job.
	return tx.Exec("DELETE FROM teldrive.files WHERE lifecycle_root = ? AND user_id = ? AND type = 'folder'", root, user).Error
}

func Expire(ctx context.Context, db *gorm.DB, now time.Time) error {
	var roots []struct {
		ID     string
		UserID int64
	}
	if err := db.WithContext(ctx).Table("teldrive.files").Select("id,user_id").Where("status IN ('trash','spam') AND id = lifecycle_root AND lifecycle_at <= ?", now.Add(-Retention)).Find(&roots).Error; err != nil {
		return err
	}
	for _, root := range roots {
		if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", root.UserID).Error; err != nil {
				return err
			}
			var count int64
			if err := tx.Table("teldrive.files").Where("id = ? AND user_id = ? AND status IN ('trash','spam') AND lifecycle_at <= ?", root.ID, root.UserID, now.Add(-Retention)).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return nil
			}
			return purgeGroup(tx, root.UserID, root.ID)
		}); err != nil {
			return err
		}
	}
	return nil
}
