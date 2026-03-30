package ladata

import (
	_ "embed"
	"fmt"

	"gopkg.in/yaml.v2"
)

//go:embed targets.yaml
var targetsYAML []byte

// Authority is a pilot local authority with a stable ONS / GSS area code (LAD or UA).
type Authority struct {
	Name     string `yaml:"name"`
	AreaCode string `yaml:"area_code"`
}

type targetsFile struct {
	Targets []Authority `yaml:"targets"`
}

// LoadTargets returns the embedded pilot authority list used for data pulls and EDA.
func LoadTargets() ([]Authority, error) {
	var f targetsFile
	if err := yaml.Unmarshal(targetsYAML, &f); err != nil {
		return nil, fmt.Errorf("ladata: parse targets.yaml: %w", err)
	}
	if len(f.Targets) == 0 {
		return nil, fmt.Errorf("ladata: no targets in targets.yaml")
	}
	return f.Targets, nil
}

// AreaCodes returns GSS codes for all pilot authorities.
func AreaCodes() ([]string, error) {
	t, err := LoadTargets()
	if err != nil {
		return nil, err
	}
	out := make([]string, len(t))
	for i := range t {
		out[i] = t[i].AreaCode
	}
	return out, nil
}
