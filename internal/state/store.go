package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"credential-priority/internal/core"
)

// SchemaVersion 是当前缓存条目的结构版本。
const SchemaVersion = 1

// ErrCorruptCache 表示缓存文件不是可解析的状态文档。
var ErrCorruptCache = errors.New("state: corrupt cache")

// Source 标识缓存条目来源，仅用于诊断与节流。
type Source string

const (
	// SourceFreshProbe 表示条目来自 fresh probe 结果。
	SourceFreshProbe Source = "fresh_probe"
)

// Entry 是 refresh-cache.json 内单个 auth_index 的状态条目。
type Entry struct {
	SchemaVersion int           `json:"schema_version"`
	Provider      core.Provider `json:"provider"`
	AuthIndex     string        `json:"auth_index"`
	ObservedAt    time.Time     `json:"observed_at"`
	ResetAt       time.Time     `json:"reset_at"`
	Remaining     int           `json:"remaining"`
	Source        Source        `json:"source"`
	LastError     string        `json:"last_error"`
	NextProbeAt   time.Time     `json:"next_probe_at"`
}

// ProbePolicy 定义状态缓存何时必须重新 fresh probe。
type ProbePolicy struct {
	TTL             time.Duration
	ResetStaleAfter time.Duration
}

// ProbeCheck 是 NeedsProbe 的输入条件。
type ProbeCheck struct {
	AuthIndex string
	Provider  core.Provider
	Now       time.Time
	Policy    ProbePolicy
}

// ProbeSuccess 是 fresh probe 成功后写入缓存的状态。
type ProbeSuccess struct {
	AuthIndex   string
	Provider    core.Provider
	ObservedAt  time.Time
	ResetAt     time.Time
	Remaining   int
	Source      Source
	NextProbeAt time.Time
}

// ProbeFailure 是 probe 失败后用于节流与诊断的状态。
type ProbeFailure struct {
	AuthIndex   string
	Provider    core.Provider
	ObservedAt  time.Time
	Err         error
	NextProbeAt time.Time
}

// Store 持有 refresh-cache.json 的内存状态，不暴露排序快照。
type Store struct {
	mu      sync.RWMutex
	path    string
	entries map[string]Entry
}

type document struct {
	Entries map[string]Entry `json:"entries"`
}

// Load 从 path 读取缓存；文件不存在时返回空 Store。
func Load(ctx context.Context, path string) (*Store, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("load state context: %w", err)
	}
	store := &Store{path: path, entries: map[string]Entry{}}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return store, nil
		}
		return nil, fmt.Errorf("read state cache %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return store, nil
	}
	var doc document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return store, fmt.Errorf("decode state cache %s: %w", path, errors.Join(ErrCorruptCache, err))
	}
	if doc.Entries != nil {
		store.entries = doc.Entries
	}
	return store, nil
}

// SaveAtomic 通过同目录临时文件加 rename 原子写入缓存文档。
func (s *Store) SaveAtomic(ctx context.Context) (err error) {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("save state context: %w", err)
	}
	s.mu.RLock()
	path := s.path
	data, err := json.MarshalIndent(document{Entries: s.entries}, "", "  ")
	s.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("encode state cache: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create state cache dir %s: %w", dir, err)
	}
	tmpPath := filepath.Join(dir, filepath.Base(path)+".tmp")
	defer func() {
		if err != nil {
			err = errors.Join(err, os.Remove(tmpPath))
		}
	}()
	if err = os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("write state cache temp: %w", err)
	}
	if err = os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename state cache temp: %w", err)
	}
	return nil
}

// MarkProbeSuccess 写入 fresh probe 成功后的节流与诊断状态。
func (s *Store) MarkProbeSuccess(ctx context.Context, success ProbeSuccess) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("mark probe success context: %w", err)
	}
	entry := Entry{
		SchemaVersion: SchemaVersion,
		Provider:      success.Provider,
		AuthIndex:     entryKey(success.AuthIndex),
		ObservedAt:    success.ObservedAt.UTC(),
		ResetAt:       success.ResetAt.UTC(),
		Remaining:     success.Remaining,
		Source:        success.Source,
		LastError:     "",
		NextProbeAt:   success.NextProbeAt.UTC(),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[entry.AuthIndex] = entry
	return nil
}

// MarkProbeFailure 写入 probe 失败后的脱敏错误与下次探测时间。
func (s *Store) MarkProbeFailure(ctx context.Context, failure ProbeFailure) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("mark probe failure context: %w", err)
	}
	key := entryKey(failure.AuthIndex)
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.entries[key]
	entry.SchemaVersion = SchemaVersion
	entry.Provider = failure.Provider
	entry.AuthIndex = key
	entry.ObservedAt = failure.ObservedAt.UTC()
	entry.LastError = sanitizeProbeError(failure.Err)
	entry.NextProbeAt = failure.NextProbeAt.UTC()
	s.entries[key] = entry
	return nil
}

// NeedsProbe 判断 auth_index 是否必须重新执行 fresh probe。
func (s *Store) NeedsProbe(ctx context.Context, check ProbeCheck) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("needs probe context: %w", err)
	}
	key := entryKey(check.AuthIndex)
	s.mu.RLock()
	entry, ok := s.entries[key]
	s.mu.RUnlock()
	if !ok {
		return true, nil
	}
	if entry.SchemaVersion != SchemaVersion {
		return true, nil
	}
	if entry.Provider != check.Provider {
		return true, nil
	}
	if isTTLExpired(entry, check) {
		return true, nil
	}
	if isResetReached(entry, check.Now) {
		return true, nil
	}
	if isResetTooOld(entry, check) {
		return true, nil
	}
	if !entry.NextProbeAt.IsZero() && check.Now.Before(entry.NextProbeAt) {
		return false, nil
	}
	if !entry.NextProbeAt.IsZero() && !check.Now.Before(entry.NextProbeAt) {
		return true, nil
	}
	return false, nil
}

// DiagnosticEntry 返回单条缓存诊断信息的副本。
func (s *Store) DiagnosticEntry(authIndex string) (Entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.entries[entryKey(authIndex)]
	return entry, ok
}

func entryKey(authIndex string) string {
	return strings.TrimSpace(authIndex)
}

func isTTLExpired(entry Entry, check ProbeCheck) bool {
	return !entry.ObservedAt.IsZero() && check.Policy.TTL > 0 && !check.Now.Before(entry.ObservedAt.Add(check.Policy.TTL))
}

func isResetReached(entry Entry, now time.Time) bool {
	return !entry.ResetAt.IsZero() && !now.Before(entry.ResetAt)
}

func isResetTooOld(entry Entry, check ProbeCheck) bool {
	return !entry.ResetAt.IsZero() && check.Policy.ResetStaleAfter > 0 && check.Now.Sub(entry.ResetAt) > check.Policy.ResetStaleAfter
}

func sanitizeProbeError(err error) string {
	if err == nil {
		return ""
	}
	text := strings.TrimSpace(err.Error())
	lower := strings.ToLower(text)
	for _, word := range []string{"authorization", "bearer", "token", "api_key", "apikey", "secret", "credential", "raw-auth", "raw auth", "auth json"} {
		if strings.Contains(lower, word) {
			return "probe failed: sensitive upstream error redacted"
		}
	}
	if len(text) > 240 {
		return text[:240]
	}
	return text
}
