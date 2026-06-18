// 图片处理管线 / Image Processing Pipeline
// 功能：magic-bytes 校验、解码、去 EXIF(再编码)、多尺寸缩放、WebP 编码、内容哈希
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-18 09:48:36
package media

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"

	"github.com/gen2brain/webp"
	xdraw "golang.org/x/image/draw"
	xwebp "golang.org/x/image/webp"
)

// ErrUnsupportedType 表示上传不是受支持的图片（magic bytes 不符）。
var ErrUnsupportedType = errors.New("media: 不支持的图片类型（仅 JPEG/PNG/WebP）")

// 派生尺寸：仅缩小不放大。
type sizeSpec struct {
	label  string
	maxDim int
}

var derivedSizes = []sizeSpec{
	{"thumb", 200},
	{"medium", 800},
	{"large", 1600},
}

// Derivative 是一张派生图的结果。
type Derivative struct {
	Label  string
	Format string
	Bytes  []byte
	Width  int
	Height int
}

// Processed 是一次图片处理的完整产物。
type Processed struct {
	ContentHash    string
	MIME           string
	Ext            string
	Width          int
	Height         int
	OriginalBytes  []byte // 已去 EXIF 的全分辨率原图（原格式）
	Derivatives    []Derivative
}

// DetectType 用 magic bytes 判定真实类型，返回 mime 与扩展名。不信任客户端 content-type。
func DetectType(data []byte) (mime, ext string, err error) {
	switch {
	case len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return "image/jpeg", "jpg", nil
	case len(data) >= 8 && bytes.Equal(data[:8], []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}):
		return "image/png", "png", nil
	case len(data) >= 12 && bytes.Equal(data[0:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return "image/webp", "webp", nil
	default:
		return "", "", ErrUnsupportedType
	}
}

// Process 校验并处理上传图片：去 EXIF 原图 + 多尺寸 WebP 派生。
func Process(data []byte) (*Processed, error) {
	mime, ext, err := DetectType(data)
	if err != nil {
		return nil, err
	}
	img, err := decode(data, mime)
	if err != nil {
		return nil, fmt.Errorf("media: 解码失败: %w", err)
	}

	sum := sha256.Sum256(data)
	out := &Processed{
		ContentHash: hex.EncodeToString(sum[:]),
		MIME:        mime,
		Ext:         ext,
		Width:       img.Bounds().Dx(),
		Height:      img.Bounds().Dy(),
	}

	// 去 EXIF 原图：解码后按原格式再编码（只留像素，元数据丢弃）。
	origBytes, err := encodeOriginal(img, mime)
	if err != nil {
		return nil, err
	}
	out.OriginalBytes = origBytes

	// 多尺寸 WebP 派生（仅缩小不放大）。
	maxOrig := max(out.Width, out.Height)
	for _, sz := range derivedSizes {
		if sz.maxDim >= maxOrig && sz.label != "large" {
			// 原图比该档还小：跳过（large 仍生成一份等比 WebP 作主图）。
			continue
		}
		scaled := resizeFit(img, sz.maxDim)
		b, err := encodeWebP(scaled, 80)
		if err != nil {
			return nil, err
		}
		out.Derivatives = append(out.Derivatives, Derivative{
			Label: sz.label, Format: "webp", Bytes: b,
			Width: scaled.Bounds().Dx(), Height: scaled.Bounds().Dy(),
		})
	}
	return out, nil
}

func decode(data []byte, mime string) (image.Image, error) {
	switch mime {
	case "image/jpeg":
		return jpeg.Decode(bytes.NewReader(data))
	case "image/png":
		return png.Decode(bytes.NewReader(data))
	case "image/webp":
		return xwebp.Decode(bytes.NewReader(data))
	default:
		return nil, ErrUnsupportedType
	}
}

func encodeOriginal(img image.Image, mime string) ([]byte, error) {
	var buf bytes.Buffer
	var err error
	switch mime {
	case "image/jpeg":
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 92})
	case "image/png":
		err = png.Encode(&buf, img)
	case "image/webp":
		err = webp.Encode(&buf, img, webp.Options{Quality: 92})
	default:
		return nil, ErrUnsupportedType
	}
	if err != nil {
		return nil, fmt.Errorf("media: 编码原图失败: %w", err)
	}
	return buf.Bytes(), nil
}

func encodeWebP(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	if err := webp.Encode(&buf, img, webp.Options{Quality: quality}); err != nil {
		return nil, fmt.Errorf("media: 编码 WebP 失败: %w", err)
	}
	return buf.Bytes(), nil
}

// resizeFit 等比缩放使最长边 <= maxDim；原图已小于则原样返回。
func resizeFit(img image.Image, maxDim int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	longest := max(w, h)
	if longest <= maxDim {
		return img
	}
	nw := w * maxDim / longest
	nh := h * maxDim / longest
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, b, xdraw.Over, nil)
	return dst
}
