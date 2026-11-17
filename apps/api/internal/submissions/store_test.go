package submissions

import (
	"bytes"
	"github.com/google/uuid"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileValidationAndRootedStorage(t *testing.T) {
	for _, tc := range []struct{ name, content string }{
		{"../x.txt", "evidence"}, {"x\\x.txt", "evidence"}, {"x\r\n.txt", "evidence"}, {"script.html", "<html>Hi</html>"}, {"x.svg", "<svg/>"}, {"image.png", "not an image"}, {"x.exe", "executable"}, {"x.txt", "<html><script>alert(1)</script></html>"}, {"binary.txt", "\x00secret"}, {"empty.txt", ""},
	} {
		if _, _, _, e := validateFile(tc.name, strings.NewReader(tc.content)); e == nil {
			t.Fatal("unsafe file accepted", tc.name)
		}
	}
	if _, _, _, e := validateFile("large.txt", bytes.NewReader(bytes.Repeat([]byte("a"), int(MaxFileBytes+1)))); e == nil {
		t.Fatal("oversize file accepted")
	}
	dir := t.TempDir()
	s, e := OpenStore(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	data, media, digest, e := validateFile("成果.md", strings.NewReader("# Evidence\nReproducible result\n"))
	if e != nil || media != "text/plain" {
		t.Fatal(media, e)
	}
	key, e := s.create(uuid.New(), data)
	if e != nil {
		t.Fatal(e)
	}
	a := Attachment{storageKey: key, ByteSize: int64(len(data)), SHA256: digest}
	f, e := s.open(a)
	if e != nil {
		t.Fatal(e)
	}
	f.Close()
	info, e := os.Stat(filepath.Join(dir, key))
	if e != nil || info.Mode().Perm() != 0600 {
		t.Fatal(info, e)
	}
	if _, e = s.root.OpenFile(key, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600); e == nil {
		t.Fatal("overwrote immutable key")
	}
	if e = s.remove("../escape"); e == nil {
		t.Fatal("accepted traversal")
	}
	outside := t.TempDir()
	team := uuid.New().String()
	if e = os.Symlink(outside, filepath.Join(dir, team)); e != nil {
		t.Fatal(e)
	}
	id, _ := uuid.Parse(team)
	if _, e = s.create(id, data); e == nil {
		t.Fatal("symlink escaped private root")
	}
	if e = os.WriteFile(filepath.Join(dir, key), []byte("tampered"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = s.open(a); e == nil {
		t.Fatal("served tampered evidence")
	}
}
