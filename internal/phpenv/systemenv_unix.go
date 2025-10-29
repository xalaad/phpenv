//go:build !windows

package phpenv

import (
	"fmt"
	"os"
	"path/filepath"

	"phpenv/internal/config"
)

func setPersistentEnvironment(cfg *config.Config, runtime ResolvedRuntime) error {
	scriptDir := filepath.Join(cfg.Root, "profile")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		return err
	}
	scriptPath := filepath.Join(scriptDir, "phpenv.sh")

	pathLine := MergePathSegments(runtime.PathAdditions, "$PATH")
	content := fmt.Sprintf(`# phpenv generated file
export PHP_ROOT=%q
export CURRENT_PHP=%q
export PATH=%s
`, cfg.Root, runtime.PHPPath, pathLine)

	if err := os.WriteFile(scriptPath, []byte(content), 0o644); err != nil {
		return err
	}

	hintPath := filepath.Join(cfg.Root, "profile", "README")
	hint := []byte("# Add the following line to your shell profile:\n#   source \"" + scriptPath + "\"\n")
	_ = os.WriteFile(hintPath, hint, 0o644)
	return nil
}
