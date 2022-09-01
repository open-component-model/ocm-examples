package substitutions

import (
	"bytes"
	"encoding/json"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/parser"
	"github.com/mandelsoft/vfs/pkg/vfs"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/utils/localize"
	"github.com/open-component-model/ocm/pkg/errors"
	"github.com/open-component-model/ocm/pkg/runtime"
)

type fileinfo struct {
	content *json.RawMessage
	json    bool
}

func Substitute(subs localize.Substitutions, fs vfs.FileSystem) error {
	files := map[string]fileinfo{}

	for i, s := range subs {
		file, err := vfs.Canonical(fs, s.FilePath, true)
		if err != nil {
			return errors.Wrapf(err, "entry %d", i)
		}

		fi, ok := files[file]
		if !ok {
			fi.content = new(json.RawMessage)
			data, err := vfs.ReadFile(fs, file)
			if err != nil {
				return errors.Wrapf(err, "entry %d: cannot read file %q", i, file)
			}
			fi.json = true
			if err = runtime.DefaultJSONEncoding.Unmarshal(data, fi.content); err != nil {
				if err = runtime.DefaultYAMLEncoding.Unmarshal(data, fi.content); err != nil {
					return errors.Wrapf(err, "entry %d: invalid YAML file %q", i, file)
				}
				fi.json = false
			}
			files[file] = fi
		}

		err = set(fi.content, s.ValuePath, s.Value)
		if err != nil {
			return errors.Wrapf(err, "entry %d: cannot substitute value", i+1)
		}
	}

	for file, fi := range files {
		marshal := runtime.DefaultYAMLEncoding.Marshal
		if fi.json {
			marshal = runtime.DefaultJSONEncoding.Marshal
		}

		data, err := marshal(fi.content)
		if err != nil {
			return errors.Wrapf(err, "cannot marshal %q after substitution ", file)
		}

		err = vfs.WriteFile(fs, file, data, vfs.ModePerm)
		if err != nil {
			return errors.Wrapf(err, "file %q", file)
		}
	}
	return nil
}

func set(content *json.RawMessage, path string, value []byte) error {
	loc, err := yaml.PathString("$." + path)
	if err != nil {
		return err
	}

	file, err := parser.ParseBytes(*content, 0)
	if err != nil {
		return err
	}

	if err := loc.ReplaceWithReader(file, bytes.NewReader(value)); err != nil {
		return err
	}

	*content = []byte(file.String())

	return nil
}
