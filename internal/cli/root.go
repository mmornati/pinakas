// Package cli wires the pinakas commands.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mmornati/pinakas/internal/board"
	"github.com/mmornati/pinakas/internal/board/github"
	"github.com/mmornati/pinakas/internal/config"
	"github.com/mmornati/pinakas/internal/state"
)

// Version is set at build time with -ldflags "-X .../cli.Version=0.2.1".
// Without it, the module version recorded by `go install ...@vX.Y.Z` is used.
var Version = ""

func version() string {
	if Version != "" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return strings.TrimPrefix(info.Main.Version, "v")
	}
	return "dev"
}

// App holds what every command needs. Tests replace the factories.
type App struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Dir    string
	Now    func() time.Time

	// NewBoard builds the board backend from the configuration.
	NewBoard func(cfg config.Config) (board.Board, error)
	// NewStore returns where the claim state lives for a repository root.
	NewStore func(root string) state.Store
}

// NewApp returns an App wired to the real environment.
func NewApp() *App {
	dir, _ := os.Getwd()
	return &App{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Dir:    dir,
		Now:    time.Now,
		NewBoard: func(cfg config.Config) (board.Board, error) {
			return github.New(cfg.Board.Owner, cfg.Board.Project, cfg.Board.StatusField)
		},
		NewStore: state.DefaultStore,
	}
}

// exitError carries a specific process exit code.
type exitError struct {
	code int
	msg  string
}

func (e *exitError) Error() string { return e.msg }

// blockedExitCode is the code agent tools read as "blocked, tell the model".
const blockedExitCode = 2

// Execute runs the CLI and returns the process exit code.
func Execute(app *App, args []string) int {
	root := app.rootCommand()
	root.SetArgs(args)
	root.SetIn(app.Stdin)
	root.SetOut(app.Stdout)
	root.SetErr(app.Stderr)
	if err := root.Execute(); err != nil {
		var ee *exitError
		if errors.As(err, &ee) {
			if ee.msg != "" {
				fmt.Fprintln(app.Stderr, ee.msg)
			}
			return ee.code
		}
		fmt.Fprintln(app.Stderr, "pinakas:", err)
		return 1
	}
	return 0
}

func (app *App) rootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "pinakas",
		Short:         "Keep AI coding agents on the board",
		Long:          "pinakas makes a Kanban board the single source of truth for human + agent teams: agents claim cards, move them and record actuals, and hooks block the steps they try to skip.",
		Version:       version(),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(
		app.initCommand(),
		app.statusCommand(),
		app.claimCommand(),
		app.moveCommand(),
		app.actualsCommand(),
		app.releaseCommand(),
		app.checkPRCommand(),
		app.checkStopCommand(),
		app.syncPRCommand(),
		app.checkCICommand(),
	)
	return root
}

// workspace is the loaded configuration and claim state for a repository.
type workspace struct {
	cfg   config.Config
	root  string
	store state.Store
}

func (app *App) workspace() (*workspace, error) {
	path, err := config.Find(app.Dir)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	root := filepath.Dir(path)
	return &workspace{cfg: cfg, root: root, store: app.NewStore(root)}, nil
}

func (app *App) board(ws *workspace) (board.Board, error) {
	return app.NewBoard(ws.cfg)
}

// findCard resolves an issue reference against the board.
func (app *App) findCard(ctx context.Context, ws *workspace, b board.Board, ref string) (board.Card, error) {
	repo, number, err := board.ParseRef(ref)
	if err != nil {
		return board.Card{}, err
	}
	if repo == "" {
		repo = ws.cfg.Board.Repo
	}
	cards, err := b.Cards(ctx)
	if err != nil {
		return board.Card{}, err
	}
	return board.Find(cards, repo, number)
}
