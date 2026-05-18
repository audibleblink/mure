package sock

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ErrFrameTooLarge is returned by ReadFrame when a line exceeds the max size.
var ErrFrameTooLarge = errors.New("sock: frame exceeds max size")

// ErrUnsupportedVersion is returned by DecodeEnvelope when v != ProtocolVersion.
var ErrUnsupportedVersion = errors.New("sock: unsupported protocol version")

// WriteFrame marshals v as JSON, appends '\n', and emits it in a single Write.
func WriteFrame(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = w.Write(b)
	return err
}

// ReadFrame reads one '\n'-terminated line from r. If the line exceeds max
// bytes (excluding the terminator), it returns ErrFrameTooLarge after
// discarding the rest of the line. The returned slice has the trailing '\n'
// stripped.
func ReadFrame(r *bufio.Reader, max int) ([]byte, error) {
	if max <= 0 {
		max = MaxFrameSize
	}
	var buf []byte
	for {
		chunk, isPrefix, err := r.ReadLine()
		if err != nil {
			return nil, err
		}
		if len(buf)+len(chunk) > max {
			// Drain the rest of the oversized line so the stream stays aligned.
			for isPrefix {
				_, isPrefix, err = r.ReadLine()
				if err != nil {
					return nil, err
				}
			}
			return nil, ErrFrameTooLarge
		}
		buf = append(buf, chunk...)
		if !isPrefix {
			return buf, nil
		}
	}
}

// envelope is the minimal shape used for routing before full decode.
type envelope struct {
	V     int    `json:"v"`
	Event string `json:"event"`
}

// DecodeEnvelope extracts the event name and version from a frame without
// fully unmarshalling it. Returns ErrUnsupportedVersion if v != ProtocolVersion.
func DecodeEnvelope(b []byte) (event string, version int, err error) {
	var e envelope
	if err = json.Unmarshal(b, &e); err != nil {
		return "", 0, err
	}
	if e.V != ProtocolVersion {
		return e.Event, e.V, fmt.Errorf("%w: got %d, want %d", ErrUnsupportedVersion, e.V, ProtocolVersion)
	}
	return e.Event, e.V, nil
}
