package zarr

import (
	"io"

	"github.com/klauspost/compress/zstd"
)

var (
	zstdEncoder, _ = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
	zstdDecoder, _ = zstd.NewReader(nil)
)

// CompressZstd compresses raw data using Zstandard.
func CompressZstd(src []byte) []byte {
	return zstdEncoder.EncodeAll(src, make([]byte, 0, len(src)/2))
}

// DecompressZstd decompresses Zstandard data.
func DecompressZstd(src []byte) ([]byte, error) {
	return zstdDecoder.DecodeAll(src, nil)
}

// CompressStream compresses an io.Reader stream into an io.Writer using Zstandard.
func CompressStream(w io.Writer, r io.Reader) error {
	zw, err := zstd.NewWriter(w)
	if err != nil {
		return err
	}
	defer zw.Close()
	_, err = io.Copy(zw, r)
	return err
}

// DecompressStream decompresses an io.Reader stream into an io.Writer using Zstandard.
func DecompressStream(w io.Writer, r io.Reader) error {
	zr, err := zstd.NewReader(r)
	if err != nil {
		return err
	}
	defer zr.Close()
	_, err = io.Copy(w, zr)
	return err
}
