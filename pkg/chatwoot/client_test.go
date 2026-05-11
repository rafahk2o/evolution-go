package chatwoot

import "testing"

func TestShouldRefreshAttachmentPreview(t *testing.T) {
	tests := []struct {
		name     string
		mimetype string
		want     bool
	}{
		{name: "image", mimetype: "image/jpeg", want: true},
		{name: "image with spaces", mimetype: " image/png ", want: true},
		{name: "video", mimetype: "video/mp4", want: true},
		{name: "audio", mimetype: "audio/ogg", want: true},
		{name: "document", mimetype: "application/pdf", want: false},
		{name: "empty", mimetype: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRefreshAttachmentPreview(tt.mimetype); got != tt.want {
				t.Fatalf("shouldRefreshAttachmentPreview(%q) = %v, want %v", tt.mimetype, got, tt.want)
			}
		})
	}
}
