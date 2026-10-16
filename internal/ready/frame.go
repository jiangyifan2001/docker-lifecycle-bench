package ready

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

const FrameSize = 24

var magic = []byte("DLBENCH1")

type Frame struct {
	Version     uint8  `json:"version"`
	Flags       uint8  `json:"flags"`
	MonotonicNS uint64 `json:"monotonicNs"`
	PID         uint32 `json:"pid"`
}

func Parse(frame []byte) (Frame, error) {
	if len(frame) != FrameSize {
		return Frame{}, fmt.Errorf("ready frame has %d bytes, want %d", len(frame), FrameSize)
	}
	if !bytes.Equal(frame[:8], magic) {
		return Frame{}, fmt.Errorf("ready frame has invalid magic %q", frame[:8])
	}
	if frame[8] != 1 {
		return Frame{}, fmt.Errorf("ready frame version %d is unsupported", frame[8])
	}
	if frame[10] != 0 || frame[11] != 0 {
		return Frame{}, fmt.Errorf("ready frame reserved bytes are non-zero")
	}
	return Frame{
		Version:     frame[8],
		Flags:       frame[9],
		MonotonicNS: binary.BigEndian.Uint64(frame[12:20]),
		PID:         binary.BigEndian.Uint32(frame[20:24]),
	}, nil
}

// ReadDockerStream extracts one probe frame from Docker's multiplexed attach stream.
func ReadDockerStream(reader io.Reader) (Frame, error) {
	var payload bytes.Buffer
	for payload.Len() < FrameSize {
		var header [8]byte
		if _, err := io.ReadFull(reader, header[:]); err != nil {
			return Frame{}, fmt.Errorf("read Docker stream header: %w", err)
		}
		stream := header[0]
		length := binary.BigEndian.Uint32(header[4:8])
		if length > 1<<20 {
			return Frame{}, fmt.Errorf("Docker stream frame is unreasonably large: %d bytes", length)
		}
		chunk := make([]byte, int(length))
		if _, err := io.ReadFull(reader, chunk); err != nil {
			return Frame{}, fmt.Errorf("read Docker stream payload: %w", err)
		}
		switch stream {
		case 1:
			remaining := FrameSize - payload.Len()
			if len(chunk) > remaining {
				chunk = chunk[:remaining]
			}
			_, _ = payload.Write(chunk)
		case 2:
			return Frame{}, fmt.Errorf("probe wrote to stderr before readiness: %q", chunk)
		default:
			return Frame{}, fmt.Errorf("unexpected Docker stream type %d", stream)
		}
	}
	return Parse(payload.Bytes())
}
