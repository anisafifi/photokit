package preset

import "gopkg.in/yaml.v3"

func yamlMarshal(p *Preset) ([]byte, error) { return yaml.Marshal(p) }
