package app

import (
	"embed"
	"net/http"
	"os"
)

//go:embed webui/index.html webui/webui.css webui/webui.js
var webUIFiles embed.FS

func serveWebUIFile(w http.ResponseWriter, name, contentType string) {
	data, err := os.ReadFile(name)
	if err != nil {
		if !os.IsNotExist(err) {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		data, err = webUIFiles.ReadFile("webui/" + name)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}

	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
