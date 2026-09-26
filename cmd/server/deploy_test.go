package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

func deployDuration(t *testing.T, path, section string, re *regexp.Regexp, unset time.Duration) time.Duration {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if section != "" {
		start := strings.Index(s, section)
		if start < 0 {
			t.Fatalf("%s: no %q section", path, section)
		}
		s = s[start+len(section):]
		if end := regexp.MustCompile(`\n  \S`).FindStringIndex(s); end != nil {
			s = s[:end[0]]
		}
	}
	m := re.FindStringSubmatch(s)
	if m == nil {
		return unset
	}
	d, err := time.ParseDuration(m[1])
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestStopGracePeriodOutlastsShutdown(t *testing.T) {
	grace := deployDuration(t, "../../deploy/docker-compose.yml", "\n  app:", regexp.MustCompile(`stop_grace_period:\s*(\S+)`), 10*time.Second)
	if want := shutdownTimeout + 5*time.Second; grace < want {
		t.Fatalf("app stop_grace_period = %s, want at least shutdownTimeout + 5s = %s", grace, want)
	}
}

func TestCaddyRetryCoversDrainAndRestart(t *testing.T) {
	try := deployDuration(t, "../../deploy/Caddyfile", "", regexp.MustCompile(`lb_try_duration\s+(\S+)`), 0)
	if want := shutdownTimeout + 10*time.Second; try < want {
		t.Fatalf("lb_try_duration = %s, want at least shutdownTimeout + 10s = %s", try, want)
	}
}
