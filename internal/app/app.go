package app

import (
	"os"
	"path/filepath"

	"phpenv/internal/config"
	"phpenv/internal/phpenv"
	"phpenv/internal/ui"
)

// App groups dependencies required for running the CLI or UI flows.
type App struct {
	cfg     *config.Config
	manager *phpenv.Manager
}

// New builds an App with configuration and manager ready.
func New() (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	manager := phpenv.NewManager(cfg)
	return &App{cfg: cfg, manager: manager}, nil
}

// Run executes the CLI if args are supplied, otherwise launches the interactive UI.
func (a *App) Run(args []string) error {
	if len(args) == 0 {
		return a.runUI()
	}
	return a.runCLI(args)
}

func (a *App) runUI() error {
	if err := ui.Run(a.manager); err != nil {
		if err == ui.ErrExitRequested {
			return nil
		}
		return err
	}
	return nil
}

func (a *App) runCLI(args []string) error {
	return runCLI(a, args)
}

func (a *App) Config() *config.Config {
	return a.cfg
}

func (a *App) Manager() *phpenv.Manager {
	return a.manager
}

// RootDirectory returns the directory the executable resides in.
func RootDirectory() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Dir(exe)
}
