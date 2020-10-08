package nanomsg

import (
	"errors"

	"go.nanomsg.org/mangos/v3"
)

// Reader implements io.Reader
type Reader struct {
	Socket mangos.Socket
}

// Writer implements io.Writer
type Writer struct {
	Socket mangos.Socket
}

func (r Reader) Read(p []byte) (int, error) {
	if r.Socket == nil {
		return 0, errors.New("Cannot receive, socket is nil")
	}
	received, err := r.Socket.Recv()
	if err != nil {
		return 0, err
	}
	if len(received) > cap(p) {
		return 0, errors.New("Buffer size is not big enough")
	}
	copy(p, received)
	return len(received), nil
}

func (w Writer) Write(p []byte) (int, error) {
	if w.Socket == nil {
		return 0, errors.New("Cannot write, socket is nil")
	}
	if err := w.Socket.Send(p); err != nil {
		return 0, err
	}
	return len(p), nil
}
