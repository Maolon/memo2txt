package cli

import (
	"encoding/json"
	"io"
)

func WriteJSON(w io.Writer, v any, indent bool) {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if indent {
		enc.SetIndent("", "  ")
	}
	_ = enc.Encode(v)
}
