package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestComposePersistsRuntimeDatabaseDirectory(t *testing.T) {
	dockerfile, err := os.ReadFile("../Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	workdirMatches := regexp.MustCompile(`(?m)^WORKDIR\s+(\S+)\s*$`).FindAllSubmatch(dockerfile, -1)
	if len(workdirMatches) == 0 {
		t.Fatal("Dockerfile must declare the runtime WORKDIR")
	}
	workdirMatch := workdirMatches[len(workdirMatches)-1]

	runtimeDBDirectory := filepath.ToSlash(filepath.Dir(filepath.Join(string(workdirMatch[1]), runtimeDBPath)))
	if runtimeDBDirectory != "/app/data" {
		t.Fatalf("runtime database directory is %q, want /app/data", runtimeDBDirectory)
	}

	compose, err := os.ReadFile("../docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	mount := regexp.MustCompile(`(?m)^\s+-\s+([a-zA-Z0-9_-]+):(/\S+)\s*$`).FindSubmatch(compose)
	if mount == nil {
		t.Fatal("compose service must mount a named volume")
	}
	if got := string(mount[2]); got != runtimeDBDirectory {
		t.Fatalf("compose mounts database volume at %q, runtime writes to %q", got, runtimeDBDirectory)
	}

	volumeKey := regexp.QuoteMeta(string(mount[1]))
	explicitVolume := regexp.MustCompile(`(?m)^  ` + volumeKey + `:\s*\n    name:\s*\S+\s*$`)
	if !explicitVolume.Match(compose) {
		t.Fatalf("compose volume %q must have an explicit stack-independent name", mount[1])
	}
}
