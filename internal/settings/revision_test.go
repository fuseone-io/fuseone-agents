package settings

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

func revisionOfValue(value string) domain.Revision {
	var s Store
	return s.Revision(Setting{Kind: "k", Name: "n", Value: []byte(value), Enabled: true}, nil, nil)
}

func TestRevision_namesWhatIsConfigured_notHowItIsWritten(t *testing.T) {
	t.Parallel()
	same := map[string][2]string{
		"key order":       {`{"a":1,"b":2}`, `{"b":2,"a":1}`},
		"spacing":         {`{"a":1}`, `{ "a" : 1 }`},
		"a null field":    {`{"a":1,"b":null}`, `{"a":1}`},
		"nested null":     {`{"o":{"a":1,"b":null}}`, `{"o":{"a":1}}`},
		"no value at all": {``, `{}`},
	}
	for name, pair := range same {
		if revisionOfValue(pair[0]) != revisionOfValue(pair[1]) {
			t.Errorf("%s moved the revision", name)
		}
	}
	different := map[string][2]string{
		"a value":        {`{"a":1}`, `{"a":2}`},
		"list order":     {`{"a":[1,2]}`, `{"a":[2,1]}`},
		"chosen nothing": {`{"surface":[]}`, `{}`},
		"a large number": {`{"a":12345678901234567890}`, `{"a":12345678901234567891}`},
	}
	for name, pair := range different {
		if revisionOfValue(pair[0]) == revisionOfValue(pair[1]) {
			t.Errorf("%s did not move the revision", name)
		}
	}
}

func TestRevision_movesWithWhatIsAroundTheValue(t *testing.T) {
	t.Parallel()
	var s Store
	base := Setting{Kind: "k", Name: "n", Value: []byte(`{}`), Enabled: true}
	off := base
	off.Enabled = false
	if s.Revision(base, nil, nil) == s.Revision(off, nil, nil) {
		t.Error("switching it off did not move the revision")
	}
	if s.Revision(base, nil, nil) == s.Revision(base, nil, map[string]string{"client": "a"}) {
		t.Error("what a read says about it did not move the revision")
	}
	if s.Revision(base, nil, nil) == s.RevisionUnreadable(base, nil) {
		t.Error("a secret that will not open reads as no secret at all")
	}
	// Not who wrote it, or when.
	written := base
	written.UpdatedBy = "usr_ben"
	if s.Revision(base, nil, nil) != s.Revision(written, nil, nil) {
		t.Error("who wrote it moved the revision")
	}
}
