package category

import (
	"path/filepath"
	"strings"
)

type Category string

const (
	Document Category = "document"
	Image    Category = "image"
	Video    Category = "video"
	Audio    Category = "audio"
	Archive  Category = "archive"
	Other    Category = "other"
)

var (
	documentExtensions = []string{"doc", "docx", "ppt", "pptx", "pps", "ppsx", "odt", "xls", "xlsx", "csv", "pdf", "txt"}
	imageExtensions    = []string{"jpg", "jpeg", "png", "gif", "bmp", "svg"}
	videoExtensions    = []string{"mp4", "webm", "mov", "avi", "m4v", "flv", "wmv", "mkv", "mpg", "mpeg", "m2v", "mpv"}
	audioExtensions    = []string{"mp3", "wav", "ogg", "m4a", "flac", "aac", "wma", "aiff", "ape", "alac", "opus", "pcm"}
	archiveExtensions  = []string{"zip", "rar", "tar", "gz", "7z", "iso", "dmg", "pkg"}
)

func GetCategory(fileName string) Category {
	fileExtension := filepath.Ext(fileName)
	if fileExtension != "" {
		fileExtension = strings.ToLower(fileExtension[1:])

	}

	if contains(documentExtensions, fileExtension) {
		return Document
	} else if contains(imageExtensions, fileExtension) {
		return Image
	} else if contains(videoExtensions, fileExtension) {
		return Video
	} else if contains(audioExtensions, fileExtension) {
		return Audio
	} else if contains(archiveExtensions, fileExtension) {
		return Archive
	} else {
		return Other
	}
}

func contains(slice []string, item string) bool {
	for _, a := range slice {
		if a == item {
			return true
		}
	}
	return false
}
