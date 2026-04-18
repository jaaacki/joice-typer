package uiembed

import (
	"strings"
	"testing"
)

func TestEmbeddedAssets_ContainsIndexHTML(t *testing.T) {
	data, err := EmbeddedAssets.ReadFile("dist/index.html")
	if err != nil {
		t.Fatalf("expected embedded dist/index.html: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("embedded index.html is empty")
	}
}

func TestEmbeddedAssets_IndexHTMLUsesRelativeAssetPaths(t *testing.T) {
	data, err := EmbeddedAssets.ReadFile("dist/index.html")
	if err != nil {
		t.Fatalf("expected embedded dist/index.html: %v", err)
	}
	html := string(data)
	for _, forbidden := range []string{
		`src="/assets/`,
		`href="/assets/`,
	} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("expected embedded index.html to avoid absolute asset path %q for file:// webview loading", forbidden)
		}
	}
	for _, required := range []string{
		`src="./assets/`,
		`href="./assets/`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("expected embedded index.html to contain relative asset path %q", required)
		}
	}
}

func TestEmbeddedAssets_IndexHTMLAvoidsCrossoriginOnLocalAssets(t *testing.T) {
	data, err := EmbeddedAssets.ReadFile("dist/index.html")
	if err != nil {
		t.Fatalf("expected embedded dist/index.html: %v", err)
	}
	html := string(data)
	if strings.Contains(html, "crossorigin") {
		t.Fatal("expected embedded index.html to avoid crossorigin attributes for local file:// webview assets")
	}
}
