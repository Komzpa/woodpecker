package local

import (
	"os"
	"strings"
)

// A workflow receives the host toolchain and locale, not the agent's credentials.
func sandboxEnviron() []string {
	var env []string
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		if key == "PATH" || key == "LANG" || key == "TZ" || strings.HasPrefix(key, "LC_") || key == "SSL_CERT_FILE" || key == "SSL_CERT_DIR" {
			env = append(env, value)
		}
	}
	return env
}
