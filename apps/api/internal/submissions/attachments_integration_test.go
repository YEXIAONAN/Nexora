//go:build integration

package submissions

import (
	"bytes"
	"github.com/google/uuid"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func (f *fixture) storage() string {
	f.t.Helper()
	dir := f.t.TempDir()
	s, e := OpenStore(dir)
	if e != nil {
		f.t.Fatal(e)
	}
	f.t.Cleanup(func() { s.Close() })
	f.sub.SetStore(s)
	return dir
}
func TestPrivateAttachmentBoundToImmutableVersion(t *testing.T) {
	f := setup(t)
	dir := f.storage()
	reviewer := f.reviewer("files")
	outsider := f.user("outsider", true)
	task := f.start(f.create(nil, 1))
	key := uuid.New()
	upload := func() (Attachment, error) {
		return f.sub.Upload(f.ctx, f.a, f.team, f.p.ID, task.ID, key, "成果.md", strings.NewReader("# Actual result\nVerified attachment\n"))
	}
	a, e := upload()
	if e != nil {
		t.Fatal(e)
	}
	retry, e := upload()
	if e != nil || retry.ID != a.ID {
		t.Fatal(retry, e)
	}
	_, e = f.sub.Upload(f.ctx, f.a, f.team, f.p.ID, task.ID, key, "成果.md", strings.NewReader("changed"))
	wantStatus(t, e, 409)
	_, _, e = f.sub.Download(f.ctx, f.a, f.team, f.p.ID, task.ID, uuid.New(), a.ID)
	wantStatus(t, e, 404)
	v, e := f.sub.Submit(f.ctx, f.a, f.team, f.p.ID, task.ID, SubmitInput{ClientRequestID: uuid.New(), ExpectedVersion: task.Version, AttachmentIDs: []uuid.UUID{a.ID}})
	if e != nil || len(v.Attachments) != 1 {
		t.Fatal(v, e)
	}
	meta, file, e := f.sub.Download(f.ctx, reviewer, f.team, f.p.ID, task.ID, v.ID, a.ID)
	if e != nil {
		t.Fatal(e)
	}
	body, e := io.ReadAll(file)
	file.Close()
	if e != nil || !strings.Contains(string(body), "Actual result") || meta.SubmissionVersionID == nil || *meta.SubmissionVersionID != v.ID {
		t.Fatal(meta, e)
	}
	_, _, e = f.sub.Download(f.ctx, outsider, f.team, f.p.ID, task.ID, v.ID, a.ID)
	wantStatus(t, e, 404)
	_, _, e = f.sub.Download(f.ctx, f.a, f.team, uuid.New(), task.ID, v.ID, a.ID)
	wantStatus(t, e, 404)
	_, e = f.sub.Discard(f.ctx, f.a, f.team, f.p.ID, task.ID, a.ID)
	wantStatus(t, e, 409)
	if _, e = f.pool.Exec(f.ctx, `UPDATE submission_attachments SET file_name='rewritten.txt' WHERE id=$1`, a.ID); e == nil {
		t.Fatal("bound metadata edited")
	}
	if e = f.sub.Sweep(f.ctx, time.Now().Add(48*time.Hour)); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(dir, a.storageKey)); e != nil {
		t.Fatal("bound evidence removed", e)
	}
	if e = os.Remove(filepath.Join(dir, a.storageKey)); e != nil {
		t.Fatal(e)
	}
	_, _, e = f.sub.Download(f.ctx, reviewer, f.team, f.p.ID, task.ID, v.ID, a.ID)
	wantStatus(t, e, 503)
	preserved, e := f.sub.Version(f.ctx, reviewer, f.team, f.p.ID, task.ID, v.ID)
	if e != nil || len(preserved.Attachments) != 1 {
		t.Fatal(preserved, e)
	}
}
func TestAttachmentScopeLimitsFailureAndRecovery(t *testing.T) {
	f := setup(t)
	dir := f.storage()
	task := f.start(f.create(nil, 1))
	other := f.start(f.create(nil, 1))
	for _, name := range []string{"../escape.txt", "/escape.txt", "x.svg", "x.html", "a.exe"} {
		if _, e := f.sub.Upload(f.ctx, f.a, f.team, f.p.ID, task.ID, uuid.New(), name, strings.NewReader("text")); e == nil {
			t.Fatal("unsafe name", name)
		}
	}
	_, e := f.sub.Upload(f.ctx, f.a, f.team, f.p.ID, task.ID, uuid.New(), "large.txt", bytes.NewReader(bytes.Repeat([]byte("a"), int(MaxFileBytes+1))))
	wantStatus(t, e, 422)
	a, e := f.sub.Upload(f.ctx, f.a, f.team, f.p.ID, task.ID, uuid.New(), "proof.txt", strings.NewReader("Proof"))
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.sub.Submit(f.ctx, f.a, f.team, f.p.ID, other.ID, SubmitInput{ClientRequestID: uuid.New(), ExpectedVersion: other.Version, AttachmentIDs: []uuid.UUID{a.ID}})
	wantStatus(t, e, 404)
	if _, e = f.sub.Discard(f.ctx, f.a, f.team, f.p.ID, task.ID, a.ID); e != nil {
		t.Fatal(e)
	}
	if e = f.sub.Sweep(f.ctx, time.Now()); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(dir, a.storageKey)); !os.IsNotExist(e) {
		t.Fatal("discarded bytes retained", e)
	}
	var removed bool
	if e = f.pool.QueryRow(f.ctx, `SELECT removed_at IS NOT NULL FROM submission_attachments WHERE id=$1`, a.ID).Scan(&removed); e != nil || !removed {
		t.Fatal(removed, e)
	}
	b, e := f.sub.Upload(f.ctx, f.a, f.team, f.p.ID, task.ID, uuid.New(), "expire.txt", strings.NewReader("Expire"))
	if e != nil {
		t.Fatal(e)
	}
	orphan, e := f.sub.store.create(f.team, []byte("Orphan after uncertain commit"))
	if e != nil {
		t.Fatal(e)
	}
	old := time.Now().Add(-25 * time.Hour)
	if e = os.Chtimes(filepath.Join(dir, orphan), old, old); e != nil {
		t.Fatal(e)
	}
	if e = f.sub.Sweep(f.ctx, time.Now().Add(25*time.Hour)); e != nil {
		t.Fatal(e)
	}
	for _, key := range []string{b.storageKey, orphan} {
		if _, e = os.Stat(filepath.Join(dir, key)); !os.IsNotExist(e) {
			t.Fatal("recoverable bytes retained", key, e)
		}
	}
	// Failed audit rolls the metadata back and cleans the newly written bytes.
	_, e = f.pool.Exec(f.ctx, `CREATE FUNCTION fail_upload_audit() RETURNS trigger AS $$ BEGIN IF NEW.action='attachment.staged' THEN RAISE EXCEPTION 'injected audit failure'; END IF; RETURN NEW; END; $$ LANGUAGE plpgsql; CREATE TRIGGER fail_upload_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION fail_upload_audit()`)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.sub.Upload(f.ctx, f.a, f.team, f.p.ID, task.ID, uuid.New(), "rollback.txt", strings.NewReader("rollback"))
	if e == nil {
		t.Fatal("audit failure did not roll back")
	}
	entries, e := os.ReadDir(filepath.Join(dir, f.team.String()))
	if e != nil || len(entries) != 0 {
		t.Fatal("failed upload left unreferenced file", entries, e)
	}
}
func TestAttachmentQuotaAndVersionTotal(t *testing.T) {
	f := setup(t)
	f.storage()
	task := f.start(f.create(nil, 1))
	ids := []uuid.UUID{}
	// Three real 9 MiB files exceed the per-version total without exceeding the file limit.
	for range 3 {
		a, e := f.sub.Upload(f.ctx, f.a, f.team, f.p.ID, task.ID, uuid.New(), "large.txt", bytes.NewReader(bytes.Repeat([]byte("a"), 9<<20)))
		if e != nil {
			t.Fatal(e)
		}
		ids = append(ids, a.ID)
	}
	_, e := f.sub.Submit(f.ctx, f.a, f.team, f.p.ID, task.ID, SubmitInput{ClientRequestID: uuid.New(), ExpectedVersion: task.Version, AttachmentIDs: ids})
	wantStatus(t, e, 422)
	// Unavailable objects still consume metadata quota until physical deletion is confirmed.
	_, e = f.pool.Exec(f.ctx, `INSERT INTO submission_attachments(team_id,project_id,task_id,uploader_id,client_request_id,storage_key,file_name,media_type,byte_size,sha256,expires_at) SELECT $1,$2,$3,$4,gen_random_uuid(),($1::uuid)::text||'/'||gen_random_uuid()::text||'.blob','unavailable.txt','text/plain',10485760,repeat('a',64),clock_timestamp()+interval '24 hours' FROM generate_series(1,50)`, f.team, f.p.ID, task.ID, f.a.UserID)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.sub.Upload(f.ctx, f.a, f.team, f.p.ID, task.ID, uuid.New(), "overquota.txt", strings.NewReader("Over quota"))
	wantStatus(t, e, 409)
}
