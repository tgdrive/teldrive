package version

import (
	"runtime"

	"github.com/tgdrive/teldrive/internal/api"
)

var (
	Version   = "development"
	CommitSHA = "unknown"
)

func VersionInfo() *api.ApiVersion {
	return &api.ApiVersion{
		Version:   Version,
		CommitSHA: CommitSHA,
		GoVersion: runtime.Version(),
		Os:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}
