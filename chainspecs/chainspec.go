package chainspecs

import (
	"embed"
	"fmt"
)

//go:embed *.json *.bin
var configFS embed.FS

var networkFile = map[string]string{
	"tiny-json": "tiny_genesis_stf.json",
	"tiny-bin":  "tiny_genesis_stf.bin",
}

func ReadBytes(network, format string) (data []byte, err error) {
	key := fmt.Sprintf("%s-%s", network, format) // e.g. "tiny-json" or "tiny-bin"
	path, ok := networkFile[key]
	if !ok {
		return nil, fmt.Errorf("unsupported chainspec: %q", key)
	}

	data, err = configFS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("embedded chainspec %q not found: %w", path, err)
	}

	return data, nil
}
