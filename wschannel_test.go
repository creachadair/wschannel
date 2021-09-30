package wschannel_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/creachadair/jrpc2/channel"
	"github.com/creachadair/wschannel"
)

var _ channel.Channel = (*wschannel.Channel)(nil)

func fixURL(url string) string {
	return "ws:" + strings.TrimPrefix(url, "http:")
}

func TestClientServer(t *testing.T) {
	const testMessage = "hello, is there anybody in there"
	const testReply = "ok"

	// Set up a listener to receive connections from the HTTP server.
	lst := wschannel.NewListener(nil)
	s := httptest.NewServer(lst)
	defer s.Close()

	// Server: Accept, and exchange a pair of messages with the client.
	go func() {
		ch, err := lst.Accept(context.Background())
		if err != nil {
			t.Errorf("Accept failed: %v", err)
			return
		}
		defer ch.Close()

		bits, err := ch.Recv()
		if err != nil {
			t.Errorf("Server Recv failed: %v", err)
		}
		if got := string(bits); got != testMessage {
			t.Errorf("Server message: got %q, want %q", got, testMessage)
		}
		if err := ch.Send([]byte(testReply)); err != nil {
			t.Errorf("Server Send failed: %v", err)
		}
	}()

	// Client: Send testMessage to the server, and read back its reply.
	ch, err := wschannel.Dial(fixURL(s.URL), nil)
	if err != nil {
		t.Fatalf("NewClient: unexpected error: %v", err)
	}

	if err := ch.Send([]byte(testMessage)); err != nil {
		t.Errorf("Client Send %q: %v", testMessage, err)
	}

	if got, err := ch.Recv(); err != nil {
		t.Errorf("Client Recv: unexpected error: %v", err)
	} else if s := string(got); s != testReply {
		t.Errorf("Client Recv: got %q, want %q", s, testReply)
	}

	if err := ch.Close(); err != nil {
		t.Errorf("Client Close: unexpected error: %v", err)
	}
	lst.Close()
}

func TestListenerErrors(t *testing.T) {
	t.Run("CheckReject", func(t *testing.T) {
		s := httptest.NewServer(wschannel.NewListener(&wschannel.ListenOptions{
			CheckAccept: func(*http.Request) (int, error) {
				return http.StatusTeapot, errors.New("failed")
			},
		}))
		defer s.Close()

		rsp, err := http.Get(s.URL)
		if err != nil {
			t.Errorf("Get failed: %v", err)
		} else if rsp.StatusCode != http.StatusTeapot {
			t.Errorf("Response status: got %v, want %v", rsp.StatusCode, http.StatusTeapot)
		}
	})
}
