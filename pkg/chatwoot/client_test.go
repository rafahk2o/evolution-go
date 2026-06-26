package chatwoot

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	instance_model "github.com/EvolutionAPI/evolution-go/pkg/instance/model"
)

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

func TestFindOrCreateConversationSerializesConcurrentCreatesForSameSource(t *testing.T) {
	var (
		mu          sync.Mutex
		getCount    int
		postCount   int
		secondGet   = make(chan struct{})
		secondGetMu sync.Once
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		switch r.Method {
		case http.MethodGet:
			getCount++
			currentGet := getCount
			currentPosts := postCount
			if currentGet == 2 {
				secondGetMu.Do(func() { close(secondGet) })
			}
			mu.Unlock()

			if currentPosts == 0 {
				select {
				case <-secondGet:
				case <-time.After(50 * time.Millisecond):
				}
			}

			mu.Lock()
			currentPosts = postCount
			mu.Unlock()
			if currentPosts > 0 {
				_ = json.NewEncoder(w).Encode([]conversationResponse{{ID: 1, Status: "open"}})
				return
			}
			_ = json.NewEncoder(w).Encode([]conversationResponse{})
		case http.MethodPost:
			postCount++
			createdID := postCount
			mu.Unlock()

			_ = json.NewEncoder(w).Encode(conversationResponse{ID: createdID, Status: "open"})
		default:
			mu.Unlock()
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	client := &Client{httpClient: server.Client()}
	settings := &instance_model.ChatwootSettings{
		URL:             server.URL,
		InboxIdentifier: "inbox-token",
	}

	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, 2)
	ids := make(chan int, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			id, err := client.findOrCreateConversation(settings, "555499812316")
			if err != nil {
				errs <- err
				return
			}
			ids <- id
		}()
	}

	close(start)
	wg.Wait()
	close(errs)
	close(ids)

	for err := range errs {
		t.Fatalf("findOrCreateConversation returned error: %v", err)
	}

	for id := range ids {
		if id != 1 {
			t.Fatalf("conversation id = %d, want 1", id)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if postCount != 1 {
		t.Fatalf("created conversations = %d, want 1", postCount)
	}
}
