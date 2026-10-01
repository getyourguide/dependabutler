package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRemoteRepoNames(t *testing.T) {
	dir := t.TempDir()
	writeRepoFile := func(name string, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile() failed: %v", err)
		}
		return path
	}

	for _, tt := range []struct {
		name     string
		repo     string
		repoFile string
		expected []string
		wantErr  bool
	}{
		{name: "repo takes precedence", repo: "single", repoFile: filepath.Join(dir, "missing.txt"), expected: []string{"single"}},
		{name: "blank lines skipped", repoFile: writeRepoFile("list.txt", "one\n\n  \r\ntwo\r\n three \n"), expected: []string{"one", "two", "three"}},
		{name: "missing file", repoFile: filepath.Join(dir, "missing.txt"), wantErr: true},
		{name: "no repos", repoFile: writeRepoFile("empty.txt", "\n \n"), wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := remoteRepoNames(tt.repo, tt.repoFile)
			if (err != nil) != tt.wantErr {
				t.Fatalf("remoteRepoNames() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("remoteRepoNames() = %q, expected %q", got, tt.expected)
			}
		})
	}
}
