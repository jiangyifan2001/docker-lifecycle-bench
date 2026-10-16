package ready

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestReadDockerStreamHandlesSplitPayload(t *testing.T) {
	frame := make([]byte, FrameSize)
	copy(frame, []byte("DLBENCH1"))
	frame[8] = 1
	binary.BigEndian.PutUint64(frame[12:20], 123456789)
	binary.BigEndian.PutUint32(frame[20:24], 42)

	stream := appendDockerChunk(nil, frame[:7])
	stream = appendDockerChunk(stream, frame[7:])
	parsed, err := ReadDockerStream(bytes.NewReader(stream))
	if err != nil {
		t.Fatalf("ReadDockerStream() error = %v", err)
	}
	if parsed.MonotonicNS != 123456789 || parsed.PID != 42 {
		t.Fatalf("parsed frame = %+v", parsed)
	}
}

func appendDockerChunk(dst, payload []byte) []byte {
	header := make([]byte, 8)
	header[0] = 1
	binary.BigEndian.PutUint32(header[4:8], uint32(len(payload)))
	dst = append(dst, header...)
	return append(dst, payload...)
}
