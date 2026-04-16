package linuxx11

import "testing"

func TestParseWMClass(t *testing.T) {
	tests := []struct {
		name        string
		xprop       string
		wantWMClass string
	}{
		{
			name:        "takes class part from wm_class pair",
			xprop:       `WM_CLASS(STRING) = "google-chrome", "Google-chrome"` + "\n",
			wantWMClass: "Google-chrome",
		},
		{
			name:        "trims single value",
			xprop:       `WM_CLASS(STRING) = "Firefox"` + "\n",
			wantWMClass: "Firefox",
		},
		{
			name:        "returns empty when absent",
			xprop:       `_GTK_APPLICATION_ID(UTF8_STRING) = "org.mozilla.firefox"` + "\n",
			wantWMClass: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseWMClass(tt.xprop); got != tt.wantWMClass {
				t.Fatalf("parseWMClass() = %q, want %q", got, tt.wantWMClass)
			}
		})
	}
}
