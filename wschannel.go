// Package wschannel implements the jrpc2 Channel interface over a websocket.
//
// For jrpc2, see https://godoc.org/github.com/creachadair/jrpc2.
// For websockets see https://datatracker.ietf.org/doc/html/rfc6455.
// This package uses the github.com/gorilla/websocket library.
package wschannel

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
)

var closeMessage = websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")

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
	return filterErr(c.c.WriteMessage(websocket.BinaryMessage, data))
}

// Recv implements the corresponding method of the Channel interface.
// The message type is not checked; either a binary or text message is
// accepted.
func (c *Channel) Recv() ([]byte, error) {
	_, bits, err := c.c.ReadMessage()
	if err != nil {
		return nil, filterErr(err)
	}
	return bits, nil
}

// Close sends a close message to the peer, then shuts down the underlying
// websocket and returns its status.
func (c *Channel) Close() error {
	// Send the message and close the connection before reporting back, to
	// ensure that a sending error does not orphan the net conn.
	err := c.c.WriteMessage(websocket.CloseMessage, closeMessage)
	cerr := c.c.Close()
	if err != nil {
		return err
	}
	if c.done != nil {
		close(c.done)
	}
	return cerr
}

func filterErr(err error) error {
	if _, ok := err.(*websocket.CloseError); ok {
		return net.ErrClosed
	}
	return err
}

// New wraps the given websocket connection to implement the Channel interface.
func New(conn *websocket.Conn) *Channel { return &Channel{c: conn} }

// Dial dials the specified websocket URL ("ws://....") with the given options
// and negotiates a client channel with the server.
func Dial(url string, opts *DialOptions) (*Channel, error) {
	conn, rsp, err := opts.dialer().Dial(url, opts.header())
	if err != nil {
		if err != websocket.ErrBadHandshake {
			return nil, fmt.Errorf("dial: %w", err)
		} else if msg, err := io.ReadAll(rsp.Body); err == nil && len(msg) != 0 {
			return nil, fmt.Errorf("dial: %s", strings.TrimSpace(string(msg)))
		}
		return nil, err
	}
	return &Channel{c: conn}, nil
}

// DialOptions are settings for a client channel. A nil *DialOptions is
// ready for use and provides default values as described.
type DialOptions struct {
	// If set, send these HTTP headers during the websocket handshake.
	Header http.Header

	// If set, use these dialer settings to connect the websocket.
	// If nil, it uses websocket.DefaultDialer.
	Dialer *websocket.Dialer
}

func (o *DialOptions) header() http.Header {
	if o == nil {
		return nil
	}
	return o.Header
}

func (o *DialOptions) dialer() *websocket.Dialer {
	if o == nil || o.Dialer == nil {
		return websocket.DefaultDialer
	}
	return o.Dialer
}
