package wschannel

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var ErrListenerClosed = errors.New("listener is closed")

// NewListener constructs a new listener with the given options.
// Use opts == nil for default settings (see ListenOptions).
// A Listener implements the http.Handler interface, and the caller can use the
// Accept method to obtain connected channels served by the handler.
func NewListener(opts *ListenOptions) *Listener {
	return &Listener{
		u:   opts.upgrader(),
		hdr: opts.header(),
		inc: make(chan *Channel, 1),
	}
}

// A Listener implements the http.Handler interface to bridge websocket to
// channels. Each connection served to the listener is made available to the
// Accept method, and its corresponding handler remains open until the channel
// is closed.
//
// After the listener is closed, no further connections will be admitted and
// any unaccepted pending connections are discarded.
type Listener struct {
	u   websocket.Upgrader
	hdr http.Header

	wg sync.WaitGroup

	mu     sync.Mutex
	inc    chan *Channel
	closed bool
}

// ServeHTTP implements the http.Handler interface. It upgrades the connection
// to a websocket, if possible, and enqueues a channel on the listener using
// the upgraded connection. Each invocation of the handler blocks until the
// corresponding channel closes.
func (lst *Listener) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	conn, err := lst.u.Upgrade(w, req, lst.hdr)
	if err != nil {
		return // Upgrade already sent a response
	}
	done, err := lst.add(conn)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	<-done // block until the channel closes
}

type completer <-chan struct{}

// add queues a new channel on conn for the Accept method. If lst is closed, it
// returns ErrListenerClosed; otherwise it returns a channel that will close
// when conn is no longer in use.
func (lst *Listener) add(conn *websocket.Conn) (completer, error) {
	lst.mu.Lock()
	defer lst.mu.Unlock()
	if lst.closed {
		return nil, ErrListenerClosed
	}

	lst.wg.Add(1)
	done := make(chan struct{})
	go func() {
		defer lst.wg.Done()
		lst.inc <- &Channel{c: conn, done: done}
		<-done
	}()
	return done, nil
}

// Accept blocks until a channel is available or ctx ends. Accept returns
// ErrListenerClosed if the listener has closed.  The caller must ensure the
// returned channel is closed.
func (lst *Listener) Accept(ctx context.Context) (*Channel, error) {
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
// and discarded. Close then blocks until all remianing accepted connections
// have closed.
func (lst *Listener) Close() error {
	lst.mu.Lock()
	defer lst.mu.Unlock()
	defer lst.wg.Wait() // wait outside the lock

	if lst.closed {
		return ErrListenerClosed
	}
	close(lst.inc)
	for ch := range lst.inc {
		ch.Close()
	}
	lst.closed = true
	return nil
}

// ListenOptions are settings for a listener. A nil *ListenOptions is ready for
// use and provides default values as described.
type ListenOptions struct {
	// If set, include these HTTP headers when negotiating a connection upgrade.
	Header http.Header

	// If set, use this connection upgrader. If omitted, default settings are used.
	Upgrader websocket.Upgrader
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
