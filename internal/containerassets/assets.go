package containerassets

import (
	"bytes"
	"compress/gzip"
	"io"
)

func BuildContextTar() (io.ReadCloser, error) {
	srcTarGz, err := embeddedSrcTarGz()
	if err != nil {
		return nil, err
	}
	if len(srcTarGz) > 0 {
		r, err := gzip.NewReader(bytes.NewReader(srcTarGz))
		if err != nil {
			return nil, err
		}
		return io.NopCloser(r), nil
	}
	return tarFromModule()
}

func WriteBuildContextGzip(w io.Writer) error {
	rc, err := tarFromModule()
	if err != nil {
		return err
	}
	defer rc.Close()

	gw := gzip.NewWriter(w)
	defer gw.Close()

	_, err = io.Copy(gw, rc)
	return err
}
