package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/bob/boomerang/internal/config"
	"github.com/bob/boomerang/internal/search"
	"github.com/bob/boomerang/internal/tui"
)

func main() {
	configPath := flag.String("config", "", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: error loading config: %v\n", err)
	}

	cwd, _ := os.Getwd()
	// Use CWD as the search root to satisfy "only go down the current cwd".
	searchRoot := cwd
	rules := search.LoadIgnoreFiles(searchRoot)
	if len(cfg.IgnorePatterns) > 0 {
		rules.LoadPatterns(cfg.IgnorePatterns)
	}
	idx := search.NewIndex()
	idx.RebuildFrom(context.Background(), searchRoot, rules, search.WalkOptions{}, nil)

	app := tui.NewApp(idx, cfg)
	p := tea.NewProgram(app, tea.WithAltScreen())
	m, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	a, ok := m.(tui.App)
	if !ok {
		return
	}
	chosen := a.Chosen()
	if chosen == nil {
		return
	}
	line := chosen.Line
	if line == 0 {
		line = a.LineNum()
	}

	// For Zed, we still try to detect the project root so it opens with full context.
	projectRoot := search.DetectRoot(searchRoot)

	// Open the chosen file in $EDITOR or zed, falling back to printing the path.
	editor := cfg.Editor
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	useZedFallback := false
	if editor == "" {
		editor = "zed"
		useZedFallback = true
	}

	var args []string
	if editor == "zed" {
		path := chosen.FilePath
		if line > 0 {
			path = fmt.Sprintf("%s:%d", path, line)
		}
		// Passing the project root helps Zed find the go.mod for gopls.
		args = []string{projectRoot, path}
	} else {
		args = []string{chosen.FilePath}
		if line > 0 {
			// Most editors accept +N to jump to a line.
			args = []string{fmt.Sprintf("+%d", line), chosen.FilePath}
		}
	}

	cmd := exec.Command(editor, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if os.IsNotExist(err) || (useZedFallback && err != nil) {
			if useZedFallback {
				fmt.Fprintf(os.Stderr, "error: 'zed' or $EDITOR not found. Please set your $EDITOR environment variable.\n")
			} else {
				fmt.Fprintf(os.Stderr, "error: editor '%s' not found. Please check your configuration or $EDITOR environment variable.\n", editor)
			}
			fmt.Println(chosen.FilePath)
			return
		}
		fmt.Fprintf(os.Stderr, "error opening editor: %v\n", err)
		os.Exit(1)
	}
}
