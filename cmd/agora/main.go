package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kelos-dev/agora/internal/agoracli"
)

func main() {
	cfg := agoracli.Config{
		BaseURL:       os.Getenv("AGORA_URL"),
		Token:         os.Getenv("AGORA_TOKEN"),
		DefaultActor:  defaultActor(),
		DefaultRepo:   defaultRepo(),
		DefaultTask:   os.Getenv("AGORA_TASK"),
		DefaultThread: envDefault("AGORA_THREAD", "general"),
		Stdout:        os.Stdout,
		Stderr:        os.Stderr,
	}
	os.Exit(agoracli.Run(context.Background(), os.Args[1:], cfg))
}

func defaultActor() string {
	for _, key := range []string{"AGORA_AGENT", "CODEX_AGENT", "USER"} {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return "agent"
}

func defaultRepo() string {
	if remote := runGit("config", "--get", "remote.origin.url"); remote != "" {
		return remote
	}
	if root := runGit("rev-parse", "--show-toplevel"); root != "" {
		return filepath.Base(root)
	}
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return filepath.Base(wd)
}

func runGit(args ...string) string {
	result, err := exec.Command("git", args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(result))
}

func envDefault(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
