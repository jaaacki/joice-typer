package uiembed

import "testing"

func TestEmbeddedAssets_ContainsIndexHTML(t *testing.T) {
	data, err := EmbeddedAssets.ReadFile("dist/index.html")
	if err != nil {
		t.Fatalf("expected embedded dist/index.html: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("embedded index.html is empty")
	}
}
