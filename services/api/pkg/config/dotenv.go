package config

import (
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// LoadDotEnv is a development convenience: it walks up from the working
// directory looking for .env.local and loads the first one it finds.
//
// Walking up matters because commands run from services/api while the
// environment file lives at the repository root — one file for every surface.
// A missing file is not an error: staging and production supply configuration
// through the environment, and godotenv never overwrites a variable that is
// already set.
func LoadDotEnv(filenames ...string) {
	if len(filenames) == 0 {
		filenames = []string{".env.local"}
	}

	dir, err := os.Getwd()
	if err != nil {
		return
	}

	for {
		for _, name := range filenames {
			candidate := filepath.Join(dir, name)
			if _, err := os.Stat(candidate); err == nil {
				_ = godotenv.Load(candidate)
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return
		}
		dir = parent
	}
}
