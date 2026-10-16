package bundle

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"docker-lifecycle-bench/internal/model"
)

type Bundle struct {
	path       string
	operations *os.File
	writer     *bufio.Writer
	encoder    *json.Encoder
	mu         sync.Mutex
	closed     bool
}

type Status struct {
	RunID      string    `json:"runId"`
	Status     string    `json:"status"`
	Message    string    `json:"message,omitempty"`
	FinishedAt time.Time `json:"finishedAt"`
}

func Create(root, campaignName string, now time.Time) (*Bundle, string, error) {
	random := make([]byte, 4)
	if _, err := io.ReadFull(rand.Reader, random); err != nil {
		return nil, "", fmt.Errorf("create run id: %w", err)
	}
	runID := fmt.Sprintf("%s-%s-%s", campaignName, now.UTC().Format("20060102T150405Z"), hex.EncodeToString(random))
	path := filepath.Join(root, runID)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, "", fmt.Errorf("create result root: %w", err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		return nil, "", fmt.Errorf("create result bundle: %w", err)
	}
	operations, err := os.OpenFile(filepath.Join(path, "operations.ndjson"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, "", fmt.Errorf("create operation log: %w", err)
	}
	writer := bufio.NewWriter(operations)
	return &Bundle{path: path, operations: operations, writer: writer, encoder: json.NewEncoder(writer)}, runID, nil
}

func (b *Bundle) Path() string { return b.path }

func (b *Bundle) WriteJSON(name string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(b.path, name), data, 0o644)
}

func (b *Bundle) WriteRaw(name string, data []byte) error {
	return os.WriteFile(filepath.Join(b.path, name), data, 0o644)
}

func (b *Bundle) WriteOperation(operation model.Operation) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return fmt.Errorf("result bundle is closed")
	}
	return b.encoder.Encode(operation)
}

func (b *Bundle) Finalize(status Status, summary any) error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return fmt.Errorf("result bundle is already finalized")
	}
	if err := b.writer.Flush(); err != nil {
		b.mu.Unlock()
		return err
	}
	if err := b.operations.Sync(); err != nil {
		b.mu.Unlock()
		return err
	}
	if err := b.operations.Close(); err != nil {
		b.mu.Unlock()
		return err
	}
	b.closed = true
	b.mu.Unlock()

	if err := b.WriteJSON("summary.json", summary); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}
	if err := b.WriteJSON("status.json", status); err != nil {
		return fmt.Errorf("write status: %w", err)
	}
	if err := b.writeChecksums(); err != nil {
		return fmt.Errorf("write checksums: %w", err)
	}
	return nil
}

func (b *Bundle) writeChecksums() error {
	entries, err := os.ReadDir(b.path)
	if err != nil {
		return err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && entry.Name() != "checksums.sha256" {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	var lines strings.Builder
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(b.path, name))
		if err != nil {
			return err
		}
		digest := sha256.Sum256(data)
		fmt.Fprintf(&lines, "%x  %s\n", digest, name)
	}
	return os.WriteFile(filepath.Join(b.path, "checksums.sha256"), []byte(lines.String()), 0o644)
}
