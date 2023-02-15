// Package wschannel implements the jrpc2 Channel interface over a websocket.
//
// For jrpc2, see https://godoc.org/github.com/creachadair/jrpc2.
// For websockets see https://datatracker.ietf.org/doc/html/rfc6455.
// This package uses the github.com/gorilla/websocket library.
package wschannel

import (
	"context"
	"errors"
	"net"
	"net/http"

	"nhooyr.io/websocket"
)

// Channel implements the jrpc2 Channel interface over a websocket.
//
// On the client side, use the Dial function to connect to a websocket endpoint
// and negotiate a connection.
//
// On the server side, you can either use NewListener to create a listener that
// plugs in to the http.Handler interface automatically, or handle the upgrade
// negotation explicitly and call New to construct a Channel.
type Channel struct {
	c    *websocket.Conn
	done chan struct{} // if not nil, closed by Close
}

// Send implements the corresponding method of the Channel interface.
// The data are transmitted as a single binary websocket message.
func (c *Channel) Send(data []byte) error {
	return filterErr(c.c.Write(context.Background(), websocket.MessageBinary, data))
}

// Recv implements the corresponding method of the Channel interface.
// The message type is not checked; either a binary or text message is
// accepted.
func (c *Channel) Recv() ([]byte, error) {
	_, bits, err := c.c.Read(context.Background())
	if err != nil {
		return nil, filterErr(err)
	}
	return bits, nil
}

// Close shuts down the websocket. The first Close triggers a websocket close
// handshake, but does not block for its completion.
func (c *Channel) Close() error {
	if c.done != nil {
		close(c.done)
		go c.c.Close(websocket.StatusNormalClosure, "bye")
		c.done = nil
	}
	return nil
}

// Done returns a channel that is closed when c is closed.
func (c *Channel) Done() <-chan struct{} { return c.done }

func filterErr(err error) error {
	if errors.Is(err, (*websocket.CloseError)(nil)) {
		return net.ErrClosed
	}
	return err
}

// New wraps the given websocket connection to implement the Channel interface.
func New(conn *websocket.Conn) *Channel {
	return &Channel{c: conn, done: make(chan struct{})}
}

// DialContext dials the specified websocket URL ("ws://....") with the given
// options and negotiates a client channel with the server.
func DialContext(ctx context.Context, url string, opts *DialOptions) (*Channel, error) {
	conn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPClient: opts.client(),
		HTTPHeader: opts.header(),
	})
	if err != nil {
		return nil, err
	}
	return New(conn), nil
}

// Dial is a shorthand for DialContext with a background context.
func Dial(url string, opts *DialOptions) (*Channel, error) {
	return DialContext(context.Background(), url, opts)
}

// DialOptions are settings for a client channel. A nil *DialOptions is
// ready for use and provides default values as described.
type DialOptions struct {
	// If non-nil, use this HTTP client instead of the default.
	HTTPClient *http.Client

	// If set, send these HTTP headers during the websocket handshake.
	Header http.Header
}

func (o *DialOptions) header() http.Header {
	if o == nil {
		return nil
	}
	return o.Header
}

func (o *DialOptions) client() *http.Client {
	if o == nil {
		return nil
	}
	return o.HTTPClient
}
