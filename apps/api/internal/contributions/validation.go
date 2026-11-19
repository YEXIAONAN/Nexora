package contributions

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
)

func invalid(field, message string) error {
	return httpx.ValidationFailed(map[string]string{field: message})
}
func bounded(s string, max int) bool {
	return utf8.ValidString(s) && utf8.RuneCountInString(s) <= max && !strings.ContainsRune(s, 0)
}
func normalizeCommand(c *Command, grant bool) error {
	c.Reason = strings.TrimSpace(c.Reason)
	min := 1
	if grant {
		min = 0
	}
	if c.ExpectedVersion < min {
		return invalid("expected_version", "is required and must be a valid resource version")
	}
	if c.ClientRequestID == uuid.Nil {
		return invalid("client_request_id", "must be a nonzero UUID")
	}
	if len(c.Reason) == 0 || !bounded(c.Reason, 4000) {
		return invalid("reason", "must contain 1–4000 characters")
	}
	return nil
}
func normalizeVersion(c *VersionCommand) error {
	if e := normalizeCommand(&c.Command, false); e != nil {
		return e
	}
	if c.CaseVersionID == uuid.Nil {
		return invalid("case_version_id", "is required")
	}
	return nil
}
func normalizeShare(in *ShareInput, remove bool) error {
	if e := normalizeVersion(&in.VersionCommand); e != nil {
		return e
	}
	if remove {
		in.ShareBPS = 0
		in.Basis = ""
		in.Statement = ""
		in.EvidenceRefs = []EvidenceReference{}
		return nil
	}
	if in.ShareBPS < 1 || in.ShareBPS > 10000 {
		return invalid("share_bps", "must be an integer between 1 and 10000")
	}
	in.Basis = strings.TrimSpace(in.Basis)
	if !bounded(in.Basis, 4000) || !bounded(in.Statement, 8000) {
		return invalid("share", "basis or statement exceeds its length limit")
	}
	if in.EvidenceRefs == nil {
		in.EvidenceRefs = []EvidenceReference{}
	}
	if len(in.EvidenceRefs) > 50 {
		return invalid("evidence_refs", "at most 50 references are permitted")
	}
	for i := range in.EvidenceRefs {
		v := &in.EvidenceRefs[i]
		v.Source = strings.TrimSpace(v.Source)
		v.Reference = strings.TrimSpace(v.Reference)
		if (v.Kind != "objective_fact" && v.Kind != "self_statement" && v.Kind != "third_party") || v.Source == "" || v.Reference == "" || !bounded(v.Source, 1000) || !bounded(v.Reference, 2000) {
			return invalid("evidence_refs", "each reference needs a supported kind, source and reference")
		}
	}
	return nil
}
func digestOf(v any) (string, error) {
	b, e := json.Marshal(v)
	if e != nil {
		return "", e
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}
func digestString(v string) string { h := sha256.Sum256([]byte(v)); return hex.EncodeToString(h[:]) }
func validDigest(v string) bool {
	if len(v) != 64 {
		return false
	}
	for _, c := range v {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func validJSONValue(v any) bool {
	switch x := v.(type) {
	case string:
		return bounded(x, 32768)
	case map[string]any:
		for k, v := range x {
			if !bounded(k, 256) || !validJSONValue(v) {
				return false
			}
		}
	case []any:
		for _, v := range x {
			if !validJSONValue(v) {
				return false
			}
		}
	}
	return true
}
