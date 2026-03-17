package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/yampug/btw/internal/config"
	"github.com/yampug/btw/internal/model"
	"github.com/yampug/btw/internal/search"
	"github.com/yampug/btw/internal/tui"
)

var (
	version = "dev"
)

func main() {
	configPath := flag.String("config", "", "path to config file")
	tabName := flag.String("t", "all", "start on a specific tab (all|classes|files|symbols|actions|text)")
	tabNameLong := flag.String("tab", "", "start on a specific tab (shorthand for -t)")
	filterExts := flag.String("f", "", "pre-apply extension filter (e.g., -f go,rs)")
	filterExtsLong := flag.String("filter", "", "pre-apply extension filter (shorthand for -f)")
	scopeStr := flag.String("s", "", "set scope (project|all)")
	scopeStrLong := flag.String("scope", "", "set scope (shorthand for -s)")
	searchPath := flag.String("p", "", "search in a specific directory (default: cwd)")
	searchPathLong := flag.String("path", "", "search in a specific directory (shorthand for -p)")
	noColor := flag.Bool("no-color", false, "disable colors")
	showVersion := flag.Bool("version", false, "print version and exit")
	
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: btw [flags] [initial-query]\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  btw                     Launch with empty search\n")
		fmt.Fprintf(os.Stderr, "  btw main.go             Launch with \"main.go\" pre-filled\n")
		fmt.Fprintf(os.Stderr, "  btw -t files main       Launch on Files tab searching \"main\"\n")
		fmt.Fprintf(os.Stderr, "  btw -f go NewMatcher    Launch filtering .go files, searching \"NewMatcher\"\n")
		fmt.Fprintf(os.Stderr, "  btw -p ~/projects/foo   Search in a specific directory\n")
	}

	flag.Parse()

	if *showVersion {
		fmt.Printf("btw version %s\n", version)
		return
	}

	if *noColor {
		os.Setenv("NO_COLOR", "1")
	}

	// Resolve shorthand/longhand flags
	if *tabNameLong != "" { tabName = tabNameLong }
	if *filterExtsLong != "" { filterExts = filterExtsLong }
	if *scopeStrLong != "" { scopeStr = scopeStrLong }
	if *searchPathLong != "" { searchPath = searchPathLong }

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: error loading config: %v\n", err)
	}

	cwd, _ := os.Getwd()
	root := cwd
	if *searchPath != "" {
		root = *searchPath
	}

	// Initial query from positional arguments
	initialQuery := ""
	if flag.NArg() > 0 {
		initialQuery = strings.Join(flag.Args(), " ")
	}

	// Parse initial state
	initState := tui.InitialState{
		Query: initialQuery,
	}

	switch strings.ToLower(*tabName) {
	case "classes": initState.Tab = model.TabClasses
	case "files":   initState.Tab = model.TabFiles
	case "symbols": initState.Tab = model.TabSymbols
	case "actions": initState.Tab = model.TabActions
	case "text":    initState.Tab = model.TabText
	default:        initState.Tab = model.TabAll
	}

	if *filterExts != "" {
		initState.Extensions = strings.Split(*filterExts, ",")
	}

	if *scopeStr != "" {
		projOnly := true
		if strings.ToLower(*scopeStr) == "all" {
			projOnly = false
		}
		initState.ProjectOnly = &projOnly
	}

	idx := search.NewIndex()
	idx.SetRoot(root)
	// Non-blocking startup: Indexing is triggered in App.Init()

	app := tui.NewApp(idx, cfg, initState)
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
	projectRoot := search.DetectRoot(root)

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
