package widget

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestImgLoaderFetch(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := range src.Pix {
		src.Pix[i] = 200
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, src)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(buf.Bytes())
	}))
	defer srv.Close()

	res := imgLoad.fetch(srv.URL + "/x.png")
	if res.err != nil {
		t.Fatalf("fetch error: %v", res.err)
	}
	if res.img == nil {
		t.Fatal("no image")
	}
	if res.img.Bounds().Dx() != 8 {
		t.Fatalf("decoded size = %v", res.img.Bounds())
	}
}
