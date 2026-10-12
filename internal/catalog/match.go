package catalog

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// MinTitleSimilarity is the title similarity a candidate needs to match (ADR-006).
const MinTitleSimilarity = 0.85

var (
	// bracketed drops "(Remastered 2011)", "[Live]" and similar.
	bracketed = regexp.MustCompile(`\([^)]*\)|\[[^\]]*\]`)
	// dashSuffix drops " - Live at Wembley", " - 2011 Remaster" and similar.
	dashSuffix = regexp.MustCompile(`\s+[-–—]\s+.*$`)
	// featuring drops "feat. X", "ft. X" and "featuring X" to the end.
	featuring = regexp.MustCompile(`\s(feat\.?|ft\.?|featuring)\s.*$`)
	// leadingThe makes "The Cure" and "Cure" equal for artists.
	leadingThe = regexp.MustCompile(`^the\s+`)
)

// apostrophes are removed rather than split on, so "Can't" equals "Cant".
var apostrophes = strings.NewReplacer("'", "", "’", "", "`", "")

// normalize lowercases, strips accents and punctuation, and collapses spaces.
func normalize(s string) string {
	// A transform.Chain keeps internal state, so it cannot be shared across goroutines.
	stripMarks := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	if t, _, err := transform.String(stripMarks, s); err == nil {
		s = t
	}
	s = apostrophes.Replace(strings.ToLower(s))
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	}), " ")
}

// NormalizeTitle reduces a track title to its comparable core.
// A title that is only a suffix, such as "(Untitled)", is kept whole.
func NormalizeTitle(s string) string {
	core := bracketed.ReplaceAllString(strings.ToLower(s), " ")
	core = dashSuffix.ReplaceAllString(core, "")
	core = featuring.ReplaceAllString(" "+core, "")
	if core = normalize(core); core != "" {
		return core
	}
	return normalize(s)
}

// NormalizeArtist reduces an artist name to its comparable core.
func NormalizeArtist(s string) string {
	s = featuring.ReplaceAllString(" "+strings.ToLower(s), "")
	return leadingThe.ReplaceAllString(normalize(s), "")
}

// Similarity is the normalized Levenshtein ratio of two strings, in [0, 1].
func Similarity(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	longest := max(len(ra), len(rb))
	if longest == 0 {
		return 1
	}
	return 1 - float64(levenshtein(ra, rb))/float64(longest)
}

func levenshtein(a, b []rune) int {
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}

// Pick returns the first candidate that matches the query: one of its
// artists equals the proposed artist after normalization, and the titles
// are at least MinTitleSimilarity alike.
func Pick(q Query, candidates []Track) (Track, bool) {
	artist := NormalizeArtist(q.Artist)
	title := NormalizeTitle(q.Title)
	for _, c := range candidates {
		if Similarity(title, NormalizeTitle(c.Name)) < MinTitleSimilarity {
			continue
		}
		for _, a := range c.Artists {
			if NormalizeArtist(a) == artist {
				return c, true
			}
		}
	}
	return Track{}, false
}
