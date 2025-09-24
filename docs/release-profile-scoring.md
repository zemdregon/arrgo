# Release Profile and Custom Format Scoring

> **Last reviewed:** 2025-03-08  
> **Status:** Implemented (automation package shipped with Servarr importers)

arrgo now ships a dedicated automation package that mirrors the release
profile/custom format ecosystem from the traditional *arr applications. The
`internal/automation/releaseprofile` package provides:

- **Reusable custom formats.** Each format bundles one or more matchers that must
  succeed before a release earns the associated score.
- **Preferences for groups and codecs.** Release groups, video codecs, and audio
  codecs can be rewarded (or penalised) with weighted preferences so operators
  can mirror their existing quality expectations.
- **Blocklists.** Any matcher can be wrapped in a blocking rule, stopping a
  release before automation attempts to grab it.
- **Sonarr/Radarr importers.** Native import helpers translate JSON exports from
  the Servarr applications into arrgo `CustomFormat` definitions and quality
  profiles so advanced grab rules migrate without being rewritten by hand.

## Package Overview

The package centres on the `Profile` type:

```go
profile := releaseprofile.Profile{
        Name:         "Remux-First",
        DefaultScore: 10,
        Preferences: []releaseprofile.Preference{
                releaseprofile.NewCodecPreference("x265", []string{"x265"}, 25, true, false),
        },
        Blocklist: []releaseprofile.BlockRule{
                {
                        Name:     "Reject CAM",
                        Matchers: []releaseprofile.Matcher{mustTitleMatcher},
                },
        },
        Formats: []releaseprofile.FormatScore{
                {
                        Format: customFormat,
                        Score:  300,
                },
        },
}

score, err := profile.Score(release)
if errors.Is(err, releaseprofile.ErrReleaseBlocked) {
        // Drop the candidate
}
```

A `Profile` evaluates blocklists first, then adds scores from preferences and
custom formats that match the candidate `Release`. Convenience constructors are
available for regular expression based group preferences, codec matches, and
release title patterns. Releases expose common metadata (title, release group,
codecs, size, languages) with room for additional fields via the `Extra` map so
future automation rules do not require schema migrations.

## Importing Servarr Custom Formats

Migration from Sonarr/Radarr is handled by the importer utilities:

```go
payload, _ := os.ReadFile("customformats.json")
formats, err := releaseprofile.ImportSonarrCustomFormats(payload)
if err != nil {
        log.Fatal(err)
}

profilePayload, _ := os.ReadFile("qualityprofile.json")
profile, err := releaseprofile.ImportSonarrQualityProfile(profilePayload, formats)
if err != nil {
        log.Fatal(err)
}
```

- `ImportSonarrCustomFormats` and `ImportRadarrCustomFormats` accept the JSON
  returned by `GET /api/v3/customformat`. Supported specifications cover release
  groups, release titles, editions, languages (including the "except language"
  toggle), release type, quality modifiers, indexer flags, source, resolution,
  and size ranges so the majority of community-supplied custom formats import
  cleanly.
- `ImportSonarrQualityProfile` maps a Sonarr quality profile (from
  `GET /api/v3/qualityprofile`) to an arrgo `Profile`, preserving the format
  scores and minimum/cut-off thresholds.

The importer recognises Servarr enum identifiers and converts them into the
string values arrgo stores on `Release` (e.g. language names, source types,
resolution strings). Unsupported specification types trigger informative
errors so operators know when a hand migration is required.

## Group and Codec Preferences

Group and codec weighting mirrors the existing *arr release profile experience:

```go
groupPref, _ := releaseprofile.NewGroupPreference("Trusted", []string{"^RARBG$"}, 150, false)
codecPref := releaseprofile.NewCodecPreference("x264", []string{"x264"}, 25, true, false)
```

Both helpers return `Preference` values that can be appended to the profile.
Multiple patterns are allowed, and the same helpers work for negative scoring by
passing `negate=true` or a negative score.

## Blocking Rules

Blocking rules reuse the same matcher interface so any combination of release
attributes can be rejected:

```go
camMatcher, _ := releaseprofile.NewTitlePreference("Reject CAM", []string{"\\bCAM\\b"}, 0, false)
profile.Blocklist = append(profile.Blocklist, releaseprofile.BlockRule{
        Name:     camMatcher.Name,
        Matchers: []releaseprofile.Matcher{camMatcher.Matcher},
})
```

When a block rule triggers, `Profile.Score` returns `ErrReleaseBlocked`, keeping
the orchestration stack from enqueuing a job for the release.

## Release Metadata Expectations

A `Release` should populate the fields that matter to your automation strategy.
The struct includes common fields (title, release group, codecs, container,
languages, indexer flags, size). Additional attributes can be set through the
`AddValues` helper if a custom format references metadata that is not yet first
class. Importers use the same mechanism to surface Servarr-specific fields so
legacy definitions continue to operate.

## Observability & Operations
- Emit metrics for score distributions, blocked releases, and automation
  decisions so operators can tune thresholds based on real data.
- Log release evaluations with the winning score and contributing preferences to
  aid debugging when automation accepts or rejects a candidate unexpectedly.
- Surface scoring summaries in the dashboard (e.g., top reasons for rejections)
  to build trust in the automation pipeline.

## Next Steps

- Wire the release profile scoring package into the download selection logic
  once the download client integrations land.
- Expose REST and dashboard surfaces so operators can manage preferences
  without editing JSON or writing Go code.
- Extend the importer to translate Radarr quality profiles when the movie
  automation flow is implemented.
- Introduce simulation tooling that runs historical releases through new
  profiles so operators can validate scoring strategies before rollout.
