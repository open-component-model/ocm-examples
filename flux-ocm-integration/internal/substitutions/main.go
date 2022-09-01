package substitutions

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/parser"
	"github.com/mandelsoft/spiff/spiffing"
	"github.com/mandelsoft/vfs/pkg/vfs"
	"github.com/open-component-model/ocm/pkg/contexts/ocm"
	ocmmeta "github.com/open-component-model/ocm/pkg/contexts/ocm/compdesc/meta/v1"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/utils"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/utils/localize"
	"github.com/open-component-model/ocm/pkg/errors"
	"github.com/open-component-model/ocm/pkg/runtime"
	"github.com/open-component-model/ocm/pkg/spiff"
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

func Configure(mappings []localize.Configuration, cursubst []localize.Substitution, cv ocm.ComponentVersionAccess, resolver ocm.ComponentVersionResolver, template []byte, config []byte, libraries []ocmmeta.ResourceReference, schemedata []byte) (localize.Substitutions, error) {
	var err error

	fmt.Println(string(config))
	if len(mappings) == 0 {
		return nil, nil
	}
	if len(config) == 0 {
		if len(schemedata) > 0 {
			err = spiff.ValidateByScheme([]byte("{}"), schemedata)
			if err != nil {
				return nil, errors.Wrapf(err, "config validation failed")
			}
		}
		if len(template) == 0 {
			return nil, nil
		}
	}

	stubs := spiff.Options{}
	for i, lib := range libraries {
		res, eff, err := utils.ResolveResourceReference(cv, lib, resolver)
		if err != nil {
			return nil, errors.ErrNotFound("library resource %s not found", lib.String())
		}
		defer eff.Close()
		m, err := res.AccessMethod()
		if err != nil {
			return nil, errors.ErrNotFound("cannot access library resource", lib.String())
		}
		data, err := m.Get()
		m.Close()
		if err != nil {
			return nil, errors.ErrNotFound("cannot access library resource", lib.String())
		}
		stubs.Add(spiff.StubData(fmt.Sprintf("spiff lib%d", i), data))
	}

	if len(schemedata) > 0 {
		err = spiff.ValidateByScheme(config, schemedata)
		if err != nil {
			return nil, errors.Wrapf(err, "validation failed")
		}
	}

	list := []interface{}{}
	for _, e := range cursubst {
		// TODO: escape spiff expressions, but should not occur, so omit it so far
		list = append(list, e)
	}
	for _, e := range mappings {
		list = append(list, e)
	}

	var temp map[string]interface{}
	if len(template) == 0 {
		temp = map[string]interface{}{
			"adjustments": list,
		}
	} else {
		if err = runtime.DefaultYAMLEncoding.Unmarshal(template, &temp); err != nil {
			return nil, errors.Wrapf(err, "cannot unmarshal template")
		}
		if _, ok := temp["adjustments"]; ok {
			return nil, errors.Newf("template may not contain 'adjustments'")
		}
		temp["adjustments"] = list
	}

	if _, ok := temp["utilities"]; !ok {
		temp["utilities"] = ""
	}

	template, err = runtime.DefaultJSONEncoding.Marshal(temp)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot marshal adjustments")
	}

	config, err = spiff.CascadeWith(spiff.TemplateData("adjustments", template), stubs, spiff.Values(config), spiff.Mode(spiffing.MODE_PRIVATE))
	if err != nil {
		return nil, errors.Wrapf(err, "error processing template")
	}

	var subst struct {
		Adjustments localize.Substitutions `json:"adjustments,omitempty"`
	}
	err = runtime.DefaultYAMLEncoding.Unmarshal(config, &subst)
	return subst.Adjustments, err
}
