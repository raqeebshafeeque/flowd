package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/raqeebshafeeque/flowd/internal/store"
	"io"
)

func (app application) repair(ctx context.Context, opts options) error {
	recovery, err := store.Repair(ctx, opts.path)
	if err != nil {
		if recovery != "" {
			return fmt.Errorf("originals retained in %q: %w", recovery, err)
		}
		return err
	}
	if opts.json {
		return json.NewEncoder(app.out).Encode(struct {
			OK       bool   `json:"ok"`
			Path     string `json:"path"`
			Recovery string `json:"recovery"`
		}{true, opts.path, recovery})
	}
	_, err = fmt.Fprintf(app.out, "OK restored %s\noriginals: %s\n", clean(opts.path), clean(recovery))
	return err
}

func printMaintenance(out io.Writer, opts options, path string) error {
	if opts.json {
		return json.NewEncoder(out).Encode(struct {
			OK   bool   `json:"ok"`
			Path string `json:"path"`
		}{true, path})
	}
	_, err := fmt.Fprintf(out, "OK %s\n", clean(path))
	return err
}
