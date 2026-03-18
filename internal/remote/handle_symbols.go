package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/yampug/btw/internal/search"
)

// symbolKindToString maps search.SymbolKind to the wire string used in the protocol.
var symbolKindToString = map[search.SymbolKind]string{
	search.SymbolFunc:     "func",
	search.SymbolType:     "type",
	search.SymbolConstant: "constant",
	search.SymbolVariable: "variable",
}

// stringToSymbolKind maps wire strings to search.SymbolKind for filtering.
var stringToSymbolKind = map[string]search.SymbolKind{
	"func":     search.SymbolFunc,
	"type":     search.SymbolType,
	"constant": search.SymbolConstant,
	"variable": search.SymbolVariable,
}

// HandleSymbols is the agent-side handler for "symbols" requests.
// It indexes the target root, extracts symbols, and streams them back.
func HandleSymbols(ctx context.Context, id int, params json.RawMessage, enc *Encoder) error {
	var p SymbolsParams
	if err := json.Unmarshal(params, &p); err != nil {
		return enc.Send(NewErrorEnvelope(id, ErrCodeInternal, fmt.Sprintf("bad symbols params: %v", err)))
	}

	if p.Root == "" {
		return enc.Send(NewErrorEnvelope(id, ErrCodeInternal, "symbols: root is required"))
	}

	// Validate kind_filter if provided.
	var filterKind search.SymbolKind
	hasFilter := false
	if p.KindFilter != "" {
		var ok bool
		filterKind, ok = stringToSymbolKind[p.KindFilter]
		if !ok {
			return enc.Send(NewErrorEnvelope(id, ErrCodeInternal,
				fmt.Sprintf("symbols: invalid kind_filter %q (valid: func, type, constant, variable)", p.KindFilter)))
		}
		hasFilter = true
	}

	// Verify root exists.
	info, err := os.Stat(p.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return enc.Send(NewNotFoundEnvelope(id, p.Root))
		}
		return enc.Send(NewInternalErrorEnvelope(id, err))
	}
	if !info.IsDir() {
		return enc.Send(NewErrorEnvelope(id, ErrCodeInternal, fmt.Sprintf("symbols: %s is not a directory", p.Root)))
	}

	// Build index and extract symbols.
	rules := search.LoadIgnoreFiles(p.Root)
	idx := search.NewIndex()
	idx.RebuildFrom(ctx, p.Root, rules, search.WalkOptions{}, nil)

	if ctx.Err() != nil {
		return enc.Send(NewCancelledEnvelope(id))
	}

	idx.ExtractSymbols()

	if ctx.Err() != nil {
		return enc.Send(NewCancelledEnvelope(id))
	}

	// Stream symbols.
	symbols := idx.Symbols()
	for _, sym := range symbols {
		select {
		case <-ctx.Done():
			return enc.Send(NewCancelledEnvelope(id))
		default:
		}

		// Apply kind filter.
		if hasFilter && sym.Kind != filterKind {
			continue
		}

		kindStr := symbolKindToString[sym.Kind]

		result := SymbolEntryResult{
			Name:      sym.Name,
			Signature: sym.Signature,
			Kind:      kindStr,
			FilePath:  sym.FilePath,
			RelPath:   sym.RelPath,
			FileName:  sym.FileName,
			Line:      sym.Line,
		}

		if err := enc.Send(Envelope{
			Method: MethodSymbolEntry,
			ID:     id,
			Result: result,
		}); err != nil {
			return err
		}
	}

	// Send done.
	return enc.Send(Envelope{
		Method: MethodSymbolEntry,
		ID:     id,
		Done:   true,
	})
}
