package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/bob/boomerang/internal/search"
	"github.com/bob/boomerang/internal/tui"
)

func main() {
	cwd, _ := os.Getwd()
	// Use CWD as the search root to satisfy "only go down the current cwd".
	searchRoot := cwd
	rules := search.LoadIgnoreFiles(searchRoot)
	idx := search.NewIndex()
	idx.RebuildFrom(context.Background(), searchRoot, rules, search.WalkOptions{})

	app := tui.NewApp(idx)
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

	// For Zed, we still try to detect the project root so it opens with full context.
	projectRoot := search.DetectRoot(searchRoot)

	// Open the chosen file in $EDITOR or zed, falling back to printing the path.
	editor := os.Getenv("EDITOR")
	useZedFallback := false
	if editor == "" {
		editor = "zed"
		useZedFallback = true
	}

	var args []string
	if editor == "zed" {
		path := chosen.FilePath
		if chosen.Line > 0 {
			path = fmt.Sprintf("%s:%d", path, chosen.Line)
		}
		// Passing the project root helps Zed find the go.mod for gopls.
		args = []string{projectRoot, path}
	} else {
		args = []string{chosen.FilePath}
		if chosen.Line > 0 {
			// Most editors accept +N to jump to a line.
			args = []string{fmt.Sprintf("+%d", chosen.Line), chosen.FilePath}
		}
	}

	cmd := exec.Command(editor, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if useZedFallback {
			// If we tried zed because $EDITOR was empty and it failed,
			// just print the path as a final fallback.
			fmt.Println(chosen.FilePath)
			return
		}
		fmt.Fprintf(os.Stderr, "error opening editor: %v\n", err)
		os.Exit(1)
	}
}
