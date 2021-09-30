package wschannel_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/creachadair/jrpc2/channel"
	"github.com/creachadair/wschannel"
	"github.com/gorilla/websocket"
)

var (
	_ channel.Channel = (*wschannel.Channel)(nil)

	upgrader websocket.Upgrader
)

func fixURL(url string) string {
	return "ws:" + strings.TrimPrefix(url, "http:")
}

type serveChannel struct {
	t   *testing.T
	run func(*wschannel.Channel) error
}

func (s *serveChannel) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	s.t.Helper()
	conn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		s.t.Errorf("Upgrading connection: %v", err)
	}
	ch := wschannel.New(conn)
	if err := s.run(ch); err != nil {
		s.t.Errorf("Run failed: %v", err)
	}
	if err := ch.Close(); err != nil {
		s.t.Errorf("Channel close: unexpected error: %v", err)
	}
}

func TestClientServer(t *testing.T) {
	const testMessage = "hello, is there anybody in there"
	const testReply = "ok"

	// Server: Read one message from the client, and sends back testReply.
	s := httptest.NewServer(&serveChannel{
		t: t,
		run: func(ch *wschannel.Channel) error {
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
			return nil
		},
	})
	defer s.Close()

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
}
