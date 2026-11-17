package submissions

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/google/uuid"
)

const (
	MaxFileBytes   int64 = 10 << 20
	TeamQuotaBytes int64 = 512 << 20
)

// Store is a single-instance private filesystem. Rooted operations prevent
// traversal, including symlink escapes. Keys are generated exclusively here.
type Store struct{ root *os.Root }

func OpenStore(dir string) (*Store, error) {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return nil, fmt.Errorf("create attachment directory: %w", e)
	}
	root, e := os.OpenRoot(dir)
	if e != nil {
		return nil, e
	}
	if e = root.Chmod(".", 0700); e != nil {
		root.Close()
		return nil, e
	}
	return &Store{root}, nil
}
func (s *Store) Close() error { return s.root.Close() }
func validateFile(name string, r io.Reader) (data []byte, media, digest string, err error) {
	invalid := func(msg string) ([]byte, string, string, error) {
		return nil, "", "", httpx.ValidationFailed(map[string]string{"file": msg})
	}
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 180 || strings.TrimSpace(name) != name || strings.ContainsAny(name, "/\\") || name == "." || strings.Contains(name, "..") || strings.ContainsFunc(name, unicode.IsControl) {
		return invalid("unsafe file name")
	}
	data, err = io.ReadAll(io.LimitReader(r, MaxFileBytes+1))
	if err != nil {
		return nil, "", "", httpx.BadRequest("could not read upload")
	}
	if len(data) == 0 || int64(len(data)) > MaxFileBytes {
		return invalid("file must be between 1 byte and 10 MiB")
	}
	media = http.DetectContentType(data)
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".txt", ".md", ".csv":
		if !strings.HasPrefix(media, "text/plain") || !utf8.Valid(data) || bytes.IndexFunc(data, func(c rune) bool { return unicode.IsControl(c) && c != '\n' && c != '\r' && c != '\t' }) >= 0 {
			return invalid("only plain UTF-8 text is allowed")
		}
		media = "text/plain"
	case ".pdf":
		if media != "application/pdf" || !bytes.HasPrefix(data, []byte("%PDF-")) || !bytes.Contains(data[max(0, len(data)-1024):], []byte("%%EOF")) {
			return invalid("invalid PDF content")
		}
	case ".png", ".jpg", ".jpeg":
		expected := "image/png"
		if ext != ".png" {
			expected = "image/jpeg"
		}
		cfg, format, e := image.DecodeConfig(bytes.NewReader(data))
		if media != expected || e != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 40000000 || (format != "png" && format != "jpeg") {
			return invalid("invalid or oversized image")
		}
	default:
		return invalid("allowed file types: txt, md, csv, pdf, png, jpg, jpeg")
	}
	sum := sha256.Sum256(data)
	digest = hex.EncodeToString(sum[:])
	return
}
func validKey(key string) bool {
	parts := strings.Split(key, "/")
	if len(parts) != 2 || !strings.HasSuffix(parts[1], ".blob") {
		return false
	}
	a, e := uuid.Parse(parts[0])
	if e != nil || a.String() != parts[0] {
		return false
	}
	b, e := uuid.Parse(strings.TrimSuffix(parts[1], ".blob"))
	return e == nil && b.String()+".blob" == parts[1]
}
func (s *Store) create(team uuid.UUID, data []byte) (key string, err error) {
	if e := s.root.MkdirAll(team.String(), 0700); e != nil {
		return "", e
	}
	key = team.String() + "/" + uuid.NewString() + ".blob"
	f, e := s.root.OpenFile(key, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return "", e
	}
	defer func() {
		if err != nil {
			_ = s.root.Remove(key)
		}
	}()
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	// Persist the directory entry before the metadata may commit.
	if err == nil {
		var d *os.File
		d, err = s.root.Open(team.String())
		if err == nil {
			err = d.Sync()
			_ = d.Close()
		}
	}
	return
}
func (s *Store) remove(key string) error {
	if !validKey(key) {
		return errors.New("invalid storage key")
	}
	e := s.root.Remove(key)
	if errors.Is(e, os.ErrNotExist) {
		return nil
	}
	return e
}
func (s *Store) open(a Attachment) (*os.File, error) {
	if !validKey(a.storageKey) {
		return nil, errors.New("invalid storage key")
	}
	f, e := s.root.Open(a.storageKey)
	if e != nil {
		return nil, e
	}
	fail := func() (*os.File, error) { f.Close(); return nil, errors.New("attachment integrity unavailable") }
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Size() != a.ByteSize {
		return fail()
	}
	h := sha256.New()
	if _, e = io.Copy(h, io.LimitReader(f, MaxFileBytes+1)); e != nil || hex.EncodeToString(h.Sum(nil)) != a.SHA256 {
		return fail()
	}
	if _, e = f.Seek(0, io.SeekStart); e != nil {
		return fail()
	}
	return f, nil
}
