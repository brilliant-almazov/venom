package http

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"
)

// runTruncated sends the request over a raw connection with a Content-Length larger than the body by n bytes, closes
// the write side after the body and reads the answer: the server meets the end of the body before the length it was
// promised, an error of reading the body.
func runTruncated(ctx context.Context, req *http.Request, body []byte, n int, insecure bool) (*http.Response, error) {
	host := req.URL.Host
	if req.URL.Port() == "" {
		if req.URL.Scheme == "https" {
			host = net.JoinHostPort(req.URL.Hostname(), "443")
		} else {
			host = net.JoinHostPort(req.URL.Hostname(), "80")
		}
	}
	d := &net.Dialer{Timeout: 30 * time.Second}
	raw, err := d.DialContext(ctx, "tcp", host)
	if err != nil {
		return nil, err
	}
	conn := raw
	if req.URL.Scheme == "https" {
		tc := tls.Client(raw, &tls.Config{ServerName: req.URL.Hostname(), InsecureSkipVerify: insecure}) //nolint:gosec // the step asks for it
		if err := tc.HandshakeContext(ctx); err != nil {
			_ = raw.Close()
			return nil, err
		}
		conn = tc
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(60 * time.Second))
	}
	var head bytes.Buffer
	fmt.Fprintf(&head, "%s %s HTTP/1.1\r\nHost: %s\r\n", req.Method, req.URL.RequestURI(), hostHeader(req))
	for k, vs := range req.Header {
		if http.CanonicalHeaderKey(k) == "Content-Length" || http.CanonicalHeaderKey(k) == "Host" {
			continue
		}
		for _, v := range vs {
			fmt.Fprintf(&head, "%s: %s\r\n", k, v)
		}
	}
	head.WriteString("Content-Length: " + strconv.Itoa(len(body)+n) + "\r\nConnection: close\r\n\r\n")
	head.Write(body)
	if _, err := conn.Write(head.Bytes()); err != nil {
		_ = conn.Close()
		return nil, err
	}
	closeWrite(conn)
	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	resp.Body = closer{ReadCloser: resp.Body, conn: conn}
	return resp, nil
}

func hostHeader(req *http.Request) string {
	if req.Host != "" {
		return req.Host
	}
	return req.URL.Host
}

// closeWrite half-closes the connection so the server sees the end of the body.
func closeWrite(c net.Conn) {
	switch t := c.(type) {
	case *net.TCPConn:
		_ = t.CloseWrite()
	case *tls.Conn:
		_ = t.CloseWrite()
	}
}

type closer struct {
	io.ReadCloser
	conn net.Conn
}

func (c closer) Close() error {
	_ = c.ReadCloser.Close()
	return c.conn.Close()
}
