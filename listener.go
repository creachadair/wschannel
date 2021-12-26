package wschannel

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/creachadair/jrpc2/channel"
	"github.com/gorilla/websocket"
)

// ErrListenerClosed is the error reported for a closed listener.
var ErrListenerClosed = errors.New("listener is closed")

// NewListener constructs a new listener with the given options.
// Use opts == nil for default settings (see ListenOptions).
// A Listener implements the http.Handler interface, and the caller can use the
// Accept method to obtain connected channels served by the handler.
func NewListener(opts *ListenOptions) *Listener {
	return &Listener{
		u:     opts.upgrader(),
		hdr:   opts.header(),
		check: opts.check(),
		inc:   make(chan *Channel, opts.maxPending()),
	}
}

// A Listener implements the http.Handler interface to bridge websocket
// requests to channels. Each connection served to the listener is made
// available to the Accept method, and its corresponding handler remains open
// until the channel is closed.
//
// After the listener is closed, no further connections will be admitted and
// any unaccepted pending connections are discarded.
type Listener struct {
	u     websocket.Upgrader
	hdr   http.Header
	check func(*http.Request) (int, error)

	mu     sync.Mutex
	inc    chan *Channel
	closed bool
}

// ServeHTTP implements the http.Handler interface. It upgrades the connection
// to a websocket, if possible, and enqueues a channel on the listener using
// the upgraded connection. Each invocation of the handler blocks until the
// corresponding channel closes.
func (lst *Listener) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Call the check hook.
	if code, err := lst.check(req); err != nil {
		if code <= 0 {
			code = http.StatusInternalServerError
		}
		http.Error(w, err.Error(), code)
		return
	}

	lst.mu.Lock()
	done := func() <-chan struct{} {
		defer lst.mu.Unlock()
		if lst.closed {
			http.Error(w, "listener is closed", http.StatusInternalServerError)
			return nil
		} else if len(lst.inc) == cap(lst.inc) {
			http.Error(w, "connection queue is full", http.StatusServiceUnavailable)
			return nil
		}

		conn, err := lst.u.Upgrade(w, req, lst.hdr)
		if err != nil {
			return nil // Upgrade already sent an error response
		}

		ch := New(conn)
		lst.inc <- ch
		return ch.done
	}()
	if done != nil {
		<-done // block until the Channel has closed
	}
}

// Accept blocks until a channel is available or ctx ends. Accept returns
// ErrListenerClosed if the listener has closed.  The caller must ensure the
// returned channel is closed. The concrete type of the channel returned is
// *wschannel.Channel.
func (lst *Listener) Accept(ctx context.Context) (channel.Channel, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case sc, ok := <-lst.inc:
		if !ok {
			return nil, ErrListenerClosed
		}
		return sc, nil
	}
}

// Close closes the listener, after which no further connections will be
// admitted, and any connections admitted but not yet accepted will be closed
// and discarded.
func (lst *Listener) Close() error {
	lst.mu.Lock()
	defer lst.mu.Unlock()

	if lst.closed {
		return ErrListenerClosed
	}
	close(lst.inc)
	var cerr error
	for ch := range lst.inc {
		if err := ch.Close(); cerr == nil {
			cerr = err
		}
	}
	lst.closed = true
	return cerr
}

// ListenOptions are settings for a listener. A nil *ListenOptions is ready for
// use and provides default values as described.
type ListenOptions struct {
	// The maximum number of unaccepted (pending) connections that will be
	// admitted by the listener. Connections in excess of this will be rejected.
	// If MaxPending ≤ 0, the default limit is 1.
	MaxPending int

	// If set, this function is called on each HTTP request received by the
	// listener, before attempting to upgrade.
	//
	// If CheckAccept reports an error, no upgrade is attempted, and the error
	// is returned to the caller.  If an error is being reported, the int value
	// is used as the HTTP status code if it is greater than 0; otherwise the
	// handler reports code 500 (server internal error).
	//
	// If CheckAccept is not set, all requests are upgraded.
	CheckAccept func(req *http.Request) (int, error)

	// If set, include these HTTP headers when negotiating a connection upgrade.
	Header http.Header

	// If set, use this connection upgrader. If omitted, default settings are used.
	Upgrader websocket.Upgrader
}

func (o *ListenOptions) maxPending() int {
	if o == nil || o.MaxPending <= 0 {
		return 1
	}
	return o.MaxPending
}

func (o *ListenOptions) check() func(*http.Request) (int, error) {
	if o == nil || o.CheckAccept == nil {
		return func(*http.Request) (int, error) { return 0, nil }
	}
	return o.CheckAccept
}

func (o *ListenOptions) header() http.Header {
	if o == nil {
		return nil
	}
	return o.Header
}

func (o *ListenOptions) upgrader() websocket.Upgrader {
	if o == nil {
		return websocket.Upgrader{}
	}
	return o.Upgrader
}
