package handler

import (
	"runtime"
	"testing"
)

// These tests only cover POSIX behavior because pathToURI/uriToPath delegate
// to go.lsp.dev/uri, which runs OS-specific logic based on runtime.GOOS.
// The go.lsp.dev/uri package has its own cross-platform test coverage.

func TestPathToURI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX test")
	}
	cases := []struct {
		path string
		want string
	}{
		{"/home/user/foo.mib", "file:///home/user/foo.mib"},
		{"/tmp/a.mib", "file:///tmp/a.mib"},
	}
	for _, tc := range cases {
		got := pathToURI(tc.path)
		if got != tc.want {
			t.Errorf("pathToURI(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestURIToPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX test")
	}
	cases := []struct {
		uri  string
		want string
	}{
		{"file:///home/user/foo.mib", "/home/user/foo.mib"},
		{"file:///tmp/a.mib", "/tmp/a.mib"},
	}
	for _, tc := range cases {
		got := uriToPath(tc.uri)
		if got != tc.want {
			t.Errorf("uriToPath(%q) = %q, want %q", tc.uri, got, tc.want)
		}
	}
}

func TestURIRoundTrip(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX test")
	}
	paths := []string{
		"/home/user/foo.mib",
		"/tmp/a.mib",
		"/var/lib/net-snmp/mibs/IF-MIB.txt",
	}
	for _, p := range paths {
		got := uriToPath(pathToURI(p))
		if got != p {
			t.Errorf("round trip %q -> %q", p, got)
		}
	}
}
