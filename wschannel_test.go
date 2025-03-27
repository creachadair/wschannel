package wschannel_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
		if _, ok := ch.(*wschannel.Channel); !ok {
			t.Errorf("Accepted channel is %T, not *Channel", ch)
		}

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
	done := ch.Done()

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
	if pkt, err := ch.Recv(); !errors.Is(err, net.ErrClosed) {
		t.Errorf("Channel receive: got %q, %v; want %v", pkt, err, net.ErrClosed)
	}
	if err := lst.Close(); err != nil {
		t.Errorf("Listener close: unexpected error: %v", err)
	}

	select {
	case <-done:
		t.Log("Channel closed signal received (OK)")
	case <-time.After(1 * time.Second):
		t.Error("Timed out waiting for close signal")
	}
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

	t.Run("QueueFull", func(t *testing.T) {
		lst := wschannel.NewListener(nil)
		defer lst.Close()

		s := httptest.NewServer(lst)
		defer s.Close()

		// The first connection should succeed, since there is a queue slot.
		c1, err := wschannel.Dial(fixURL(s.URL), nil)
		if err != nil {
			t.Fatalf("Client 1 failed: %v", err)
		}
		defer c1.Close()

		// The second connection should fail, since the queue is full.
		c2, err := wschannel.Dial(fixURL(s.URL), nil)
		if err != nil {
			t.Logf("Client 2 dial: got expected error: %v", err)
		} else {
			t.Errorf("Client 2 dial: got %+v, want error", c2)
			c2.Close()
		}
	})
}
