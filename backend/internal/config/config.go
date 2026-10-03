// Package config loads ai-compare settings from the environment.
//
// The repo has a single .env at its root. Inside Docker, compose injects it as
// environment variables. When the backend runs on the host (go run), Load finds
// that file by walking up from the working directory. Variables already set in
// the environment always win over the file.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	AppPort      int
	ProxyPort    int
	DatabaseURL  string
	OpenAIKey    string
	AnthropicKey string
	LocalBaseURL string
	ModelsDevURL string
	// CatalogProviders are the models.dev providers kept from the catalogue.
	CatalogProviders []string
	// DataDir holds files the app keeps between restarts, such as the catalogue cache.
	DataDir string
	// StaticDir serves the built frontend when set (the Docker image sets it).
	StaticDir string
	// AgentNetwork is the Docker network agent containers join; it only reaches api's proxy.
	AgentNetwork string
	// EnvFile is the .env that was loaded, empty if none was found.
	EnvFile string
}

func Load() (Config, error) {
	envFile, err := loadDotEnv()
	if err != nil {
		return Config{}, err
	}

	appPort, err := intVar("APP_PORT", 4700)
	if err != nil {
		return Config{}, err
	}
	proxyPort, err := intVar("PROXY_PORT", 4701)
	if err != nil {
		return Config{}, err
	}

	return Config{
		AppPort:          appPort,
		ProxyPort:        proxyPort,
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		OpenAIKey:        os.Getenv("OPENAI_API_KEY"),
		AnthropicKey:     os.Getenv("ANTHROPIC_API_KEY"),
		LocalBaseURL:     os.Getenv("LOCAL_MODELS_BASE_URL"),
		ModelsDevURL:     stringVar("MODELS_DEV_URL", "https://models.dev/api.json"),
		CatalogProviders: listVar("CATALOG_PROVIDERS", "openai,anthropic"),
		DataDir:          stringVar("DATA_DIR", "data"),
		StaticDir:        os.Getenv("STATIC_DIR"),
		AgentNetwork:     stringVar("AGENT_NETWORK", "ai-compare-agents"),
		EnvFile:          envFile,
	}, nil
}

func stringVar(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// listVar reads a comma-separated list, ignoring blanks.
func listVar(key, fallback string) []string {
	var out []string
	for _, v := range strings.Split(stringVar(key, fallback), ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func intVar(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number, got %q", key, v)
	}
	return n, nil
}

// loadDotEnv looks for .env in the working directory and its parents and sets
// every variable that is not already present in the environment.
func loadDotEnv() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		path := filepath.Join(dir, ".env")
		if _, err := os.Stat(path); err == nil {
			return path, applyDotEnv(path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}

func applyDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		key, value, ok := strings.Cut(text, "=")
		if !ok {
			return fmt.Errorf("%s:%d: missing \"=\"", path, line)
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if _, set := os.LookupEnv(key); !set {
			os.Setenv(key, value)
		}
	}
	return scanner.Err()
}
