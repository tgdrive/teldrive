package constants

type FileStatus string

const (
	FileStatusActive FileStatus = "active"

	FileStatusPendingDeletion FileStatus = "pending_deletion"
)

func (s FileStatus) String() string {
	return string(s)
}
