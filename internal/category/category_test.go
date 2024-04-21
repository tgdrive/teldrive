//gen test suite

package category

import (
	"testing"
)

func TestGetCategory(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		want     Category
	}{
		{
			name:     "Document",
			fileName: "file.doc",
			want:     Document,
		},
		{
			name:     "Image",
			fileName: "file.jpg",
			want:     Image,
		},
		{
			name:     "Video",
			fileName: "file.mp4",
			want:     Video,
		},
		{
			name:     "Audio",
			fileName: "file.mp3",
			want:     Audio,
		},
		{
			name:     "Archive",
			fileName: "file.zip",
			want:     Archive,
		},
		{
			name:     "Other",
			fileName: "file",
			want:     Other,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetCategory(tt.fileName); got != tt.want {
				t.Errorf("GetCategory() = %v, want %v", got, tt.want)
			}
		})
	}
}
