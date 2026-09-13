package ordercheck

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"

	"ytreverse/internal/youtube"
)

const (
	minCoverage = 0.8
	minRatio    = 0.95
)

type Signal struct {
	Pattern   string  `json:"pattern,omitempty"`
	Coverage  float64 `json:"coverage"`
	Up        int     `json:"up"`
	Down      int     `json:"down"`
	Equal     int     `json:"equal"`
	Direction string  `json:"direction"`
	Ratio     float64 `json:"ratio"`
}

type Result struct {
	Verdict   string `json:"verdict"`
	Basis     string `json:"basis"`
	Title     Signal `json:"title"`
	Date      Signal `json:"date"`
	Anomalies []int  `json:"anomalies"`
}

var patterns = []struct {
	name string
	re   *regexp.Regexp
}{
	{"SxxExx", regexp.MustCompile(`(?i)\bS(\d+)\s*E(\d+)`)},
	{"EP", regexp.MustCompile(`(?i)\bEP?\.?\s*(\d+)`)},
	{"Episode", regexp.MustCompile(`(?i)\bEpisode\s*(\d+)`)},
	{"화/회/편/부", regexp.MustCompile(`(\d+)\s*(?:화|회|편|부)`)},
	{"#", regexp.MustCompile(`#(\d+)`)},
	{"Part", regexp.MustCompile(`(?i)\bPart\s*(\d+)`)},
	{"Vol", regexp.MustCompile(`(?i)\bVol\.?\s*(\d+)`)},
	{"[n]", regexp.MustCompile(`\[(\d+)\]`)},
}

func Check(items []youtube.Item) Result {
	name, keys := extract(items)
	title, titleAnomalies := measure(len(items),
		func(i int) bool { return keys[i] != nil },
		func(a, b int) int { return slices.Compare(keys[a], keys[b]) })
	title.Pattern = name
	date, dateAnomalies := measure(len(items),
		func(i int) bool { return items[i].PublishedAt != nil },
		func(a, b int) int { return items[a].PublishedAt.Compare(*items[b].PublishedAt) })

	r := Result{Title: title, Date: date}
	t, d := confident(title), confident(date)
	switch {
	case t && d && title.Direction != date.Direction:
		r.Verdict, r.Anomalies = "mixed", titleAnomalies
	case t:
		r.Verdict, r.Basis, r.Anomalies = title.Direction, "title", titleAnomalies
	case d:
		r.Verdict, r.Basis, r.Anomalies = date.Direction, "date", dateAnomalies
	case title.Coverage >= minCoverage && title.Direction != "":
		r.Verdict, r.Basis, r.Anomalies = "mixed", "title", titleAnomalies
	case date.Coverage >= minCoverage && date.Direction != "":
		r.Verdict, r.Basis, r.Anomalies = "mixed", "date", dateAnomalies
	default:
		r.Verdict = "unknown"
	}
	return r
}

func Episodes(items []youtube.Item) (string, []string) {
	name, keys := extract(items)
	eps := make([]string, len(keys))
	for i, k := range keys {
		switch len(k) {
		case 1:
			eps[i] = strconv.Itoa(k[0])
		case 2:
			eps[i] = fmt.Sprintf("S%dE%d", k[0], k[1])
		}
	}
	return name, eps
}

func confident(s Signal) bool {
	return s.Coverage >= minCoverage && s.Ratio >= minRatio
}

func extract(items []youtube.Item) (string, [][]int) {
	best, bestCount := -1, 0
	for pi, p := range patterns {
		n := 0
		for _, it := range items {
			if p.re.MatchString(it.Title) {
				n++
			}
		}
		if n > bestCount {
			best, bestCount = pi, n
		}
	}
	keys := make([][]int, len(items))
	if best < 0 {
		return "", keys
	}
	for i, it := range items {
		m := patterns[best].re.FindStringSubmatch(it.Title)
		if m == nil {
			continue
		}
		key := make([]int, 0, len(m)-1)
		for _, s := range m[1:] {
			n, _ := strconv.Atoi(s)
			key = append(key, n)
		}
		keys[i] = key
	}
	return patterns[best].name, keys
}

func measure(n int, has func(int) bool, cmp func(a, b int) int) (Signal, []int) {
	type step struct{ pos, dir int }
	var s Signal
	var steps []step
	matched, prev := 0, -1
	for i := 0; i < n; i++ {
		if !has(i) {
			continue
		}
		matched++
		if prev >= 0 {
			d := cmp(prev, i)
			switch {
			case d < 0:
				s.Up++
			case d > 0:
				s.Down++
			default:
				s.Equal++
			}
			steps = append(steps, step{i, d})
		}
		prev = i
	}
	if n > 0 {
		s.Coverage = float64(matched) / float64(n)
	}
	moving := s.Up + s.Down
	if moving == 0 {
		return s, nil
	}
	want := -1
	s.Direction, s.Ratio = "asc", float64(s.Up)/float64(moving)
	if s.Down > s.Up {
		want = 1
		s.Direction, s.Ratio = "desc", float64(s.Down)/float64(moving)
	}
	var anomalies []int
	for _, st := range steps {
		if st.dir != 0 && st.dir != want {
			anomalies = append(anomalies, st.pos)
		}
	}
	return s, anomalies
}
