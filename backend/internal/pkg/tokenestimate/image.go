package tokenestimate

import (
	"encoding/base64"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"strings"

	"github.com/tidwall/gjson"
	_ "golang.org/x/image/webp"
)

func imageDimensions(value gjson.Result) (int64, int64) {
	data := value.Get("source.data").Str
	if data == "" {
		url := value.Get("image_url.url").Str
		if url == "" {
			url = value.Get("image_url").Str
		}
		if strings.HasPrefix(url, "data:image/") {
			header, encoded, ok := strings.Cut(url, ",")
			if ok && strings.HasSuffix(header, ";base64") {
				data = encoded
			}
		}
	}
	if data != "" {
		// Read bounded headers only: never decode image pixels or fetch remote URLs.
		reader := io.LimitReader(base64.NewDecoder(base64.StdEncoding, strings.NewReader(data)), 64<<10)
		if cfg, _, err := image.DecodeConfig(reader); err == nil {
			return int64(cfg.Width), int64(cfg.Height)
		}
	}
	return value.Get("width").Int(), value.Get("height").Int()
}
