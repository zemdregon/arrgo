package releaseprofile

import "testing"

func TestProfileScoring(t *testing.T) {
	groupPref, err := NewGroupPreference("RARBG", []string{"^RARBG$"}, 100, false)
	if err != nil {
		t.Fatalf("group preference error: %v", err)
	}
	codecPref := NewCodecPreference("x264", []string{"x264"}, 50, true, false)

	titleMatcher, err := newRegexMatcher(FieldTitle, []string{"\\bREMUX\\b"}, false)
	if err != nil {
		t.Fatalf("title matcher error: %v", err)
	}
	format := CustomFormat{Name: "Remux", Matchers: []Matcher{titleMatcher}}

	blockMatcher, err := newRegexMatcher(FieldTitle, []string{"\\bCAM\\b"}, false)
	if err != nil {
		t.Fatalf("block matcher error: %v", err)
	}

	profile := Profile{
		Name:         "HD",
		DefaultScore: 10,
		Preferences:  []Preference{groupPref, codecPref},
		Blocklist: []BlockRule{{
			Name:     "Block CAM",
			Matchers: []Matcher{blockMatcher},
		}},
		Formats: []FormatScore{{
			Format: format,
			Score:  200,
		}},
	}

	release := Release{
		Title:      "Example REMUX 1080p",
		Group:      "RARBG",
		VideoCodec: "x264",
	}

	score, err := profile.Score(release)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := 10 + 100 + 50 + 200
	if int(score) != want {
		t.Fatalf("unexpected score: got %.0f want %d", score, want)
	}
}

func TestProfileBlocklist(t *testing.T) {
	blockMatcher, err := newRegexMatcher(FieldTitle, []string{"\\bCAM\\b"}, false)
	if err != nil {
		t.Fatalf("block matcher error: %v", err)
	}

	profile := Profile{
		Blocklist: []BlockRule{{
			Name:     "Block CAM",
			Matchers: []Matcher{blockMatcher},
		}},
	}

	release := Release{Title: "Example CAM Release"}
	if _, err := profile.Score(release); err != ErrReleaseBlocked {
		t.Fatalf("expected ErrReleaseBlocked, got %v", err)
	}
}

func TestImportSonarrCustomFormats(t *testing.T) {
	payload := []byte(`[
        {
            "id": 1,
            "name": "Preferred Remux",
            "specifications": [
                {
                    "name": "Group",
                    "implementation": "ReleaseGroupSpecification",
                    "negate": false,
                    "required": true,
                    "fields": {"value": "^RARBG$"}
                },
                {
                    "name": "Title",
                    "implementation": "ReleaseTitleSpecification",
                    "negate": false,
                    "required": true,
                    "fields": {"value": "\\bREMUX\\b"}
                }
            ]
        }
    ]`)

	formats, err := ImportSonarrCustomFormats(payload)
	if err != nil {
		t.Fatalf("import error: %v", err)
	}
	if len(formats) != 1 {
		t.Fatalf("unexpected format count: %d", len(formats))
	}

	release := Release{Title: "Example REMUX", Group: "RARBG"}
	if !formats[0].Matches(release) {
		t.Fatalf("expected format to match release")
	}
}

func TestImportSonarrQualityProfile(t *testing.T) {
	formats := []CustomFormat{{
		ID:       7,
		Name:     "Preferred Remux",
		Matchers: []Matcher{&stringMatcher{field: FieldGroup, kind: matchExact, values: []string{"rarbg"}, caseInsensitive: true}},
	}}

	payload := []byte(`{
        "name": "HD",
        "minFormatScore": 0,
        "cutoffFormatScore": 100,
        "minUpgradeFormatScore": 10,
        "formatItems": [
            {"format": 7, "name": "Preferred Remux", "score": 300}
        ]
    }`)

	profile, err := ImportSonarrQualityProfile(payload, formats)
	if err != nil {
		t.Fatalf("import error: %v", err)
	}

	if len(profile.Formats) != 1 {
		t.Fatalf("unexpected format binding count: %d", len(profile.Formats))
	}

	release := Release{Group: "RARBG"}
	score, err := profile.Score(release)
	if err != nil {
		t.Fatalf("unexpected error scoring release: %v", err)
	}
	if score != 300 {
		t.Fatalf("unexpected score: got %.0f want 300", score)
	}
}
