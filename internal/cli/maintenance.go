package cli

import (
	"encoding/json"
	"fmt"
	"io"
)

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
