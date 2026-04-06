package common

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func LoadEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		v = strings.TrimSpace(v)
		v = strings.Trim(v, "\"'")
		if _, exists := os.LookupEnv(k); !exists {
			if err := os.Setenv(k, v); err != nil {
				return fmt.Errorf("set env %s: %w", k, err)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan env file %s: %w", path, err)
	}
	return nil
}

func LoadPreferredEnvFile() error {
	env := strings.ToLower(strings.TrimSpace(firstNonEmpty(
		os.Getenv("APP_ENV"),
		os.Getenv("ENVIRONMENT"),
		os.Getenv("GO_ENV"),
	)))

	switch env {
	case "prod", "production":
		return LoadEnvFile(".env.production")
	default:
		if err := LoadEnvFile(".env.local"); err != nil {
			return err
		}
		return LoadEnvFile(".env")
	}
}

func IsEnvReference(val string) bool {
	return envVarRe.MatchString(strings.TrimSpace(val))
}

func ResolveOptionalEnv(val string) (string, bool) {
	val = strings.TrimSpace(val)
	if val == "" {
		return "", false
	}
	resolved := strings.TrimSpace(ResolveEnv(val))
	if IsEnvReference(val) && resolved == val {
		return "", false
	}
	if resolved == "" {
		return "", false
	}
	return resolved, true
}

func LooksLikePlaceholderSecret(val string) bool {
	trimmed := strings.TrimSpace(val)
	if trimmed == "" || IsEnvReference(trimmed) {
		return true
	}

	lower := strings.ToLower(trimmed)
	switch lower {
	case "your-api-key-here",
		"replace-with-a-32-char-secret",
		"replace-with-your-model-api-key",
		"replace-with-your-embedding-api-key",
		"changeme",
		"change-me",
		"your-password",
		"your-secret",
		"your-jwt-secret":
		return true
	}

	for _, prefix := range []string{
		"replace-with",
		"your-",
		"example",
		"<",
	} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
