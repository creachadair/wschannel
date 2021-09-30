// Package wschannel implements the jrpc2 Channel interface over a websocket.
package wschannel

import (
	"net/http"

	"github.com/gorilla/websocket"
)

var closeMessage = websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")

// Channel implements the jrpc2 Channel interface over a websocket.
//
// On the server side, use wschannel.New to wrap the websocket connection.
// On the client side, use wschannel.Dial to dial a server.
type Channel struct {
	c *websocket.Conn
}

// Send implements the corresponding method of the Channel interface.
// The data are transmitted as a single binary websocket message.
func (c *Channel) Send(data []byte) error {
	return c.c.WriteMessage(websocket.BinaryMessage, data)
}

// Recv implements the corresponding method of the Channel interface.
// The message type is not checked; either a binary or text message is
// accepted.
func (c *Channel) Recv() ([]byte, error) {
	_, bits, err := c.c.ReadMessage()
	if err != nil {
		return nil, err
	}
	return bits, err
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
	return cerr
}

// New wraps the given websocket connection to implement the Channel interface.
func New(conn *websocket.Conn) *Channel { return &Channel{c: conn} }

// Dial dials the specified websocket URL ("ws://....") with the given options
// and negotiates a client channel with the server.
func Dial(url string, opts *ClientOptions) (*Channel, error) {
	conn, _, err := opts.dialer().Dial(url, opts.header())
	if err != nil {
		return nil, err
	}
	return &Channel{c: conn}, nil
}

// ClientOptions are settings for a client channel. A nil *ClientOptions is
// ready for use and provides default values as described.
type ClientOptions struct {
	// If set, send these HTTP headers during the websocket handshake.
	Header http.Header

	// If set, use these dialer settings to connect the websocket.
	// If nil, it uses websocket.DefaultDialer.
	Dialer *websocket.Dialer
}

func (o *ClientOptions) header() http.Header {
	if o == nil {
		return nil
	}
	return o.Header
}

func (o *ClientOptions) dialer() *websocket.Dialer {
	if o == nil || o.Dialer == nil {
		return websocket.DefaultDialer
	}
	return o.Dialer
}
