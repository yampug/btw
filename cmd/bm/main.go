package main

import (
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/bob/boomerang/internal/tui"
)

func main() {
	app := tui.NewApp()
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

	// Open the chosen file in $EDITOR, falling back to printing the path.
	editor := os.Getenv("EDITOR")
	if editor == "" {
		fmt.Println(chosen.FilePath)
		return
	}

	args := []string{chosen.FilePath}
	if chosen.Line > 0 {
		// Most editors accept +N to jump to a line.
		args = []string{fmt.Sprintf("+%d", chosen.Line), chosen.FilePath}
	}
	cmd := exec.Command(editor, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error opening editor: %v\n", err)
		os.Exit(1)
	}
}
