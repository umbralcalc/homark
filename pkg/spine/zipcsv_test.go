package spine

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractLargestCSVFromZip(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "in.zip")
	zf, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(zf)
	w1, err := zw.Create("small.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w1.Write([]byte("a,b\n1,2\n")); err != nil {
		t.Fatal(err)
	}
	w2, err := zw.Create("Data/big.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w2.Write([]byte("pcds,lad23cd\nAB1 2CD,E09000030\n")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zf.Close(); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(dir, "out.csv")
	if err := ExtractLargestCSVFromZip(zipPath, out); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw[:4]) != "pcds" {
		t.Fatalf("got %q", string(raw))
	}
}
