module github.com/linlay/cli-imagine

go 1.26

require (
	github.com/pelletier/go-toml/v2 v2.2.4
	github.com/santhosh-tekuri/jsonschema/v5 v5.0.0
	github.com/spf13/cobra v1.10.1
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/spf13/cobra => ./third_party/cobra

replace github.com/santhosh-tekuri/jsonschema/v5 => ./third_party/jsonschema

replace gopkg.in/check.v1 => ./third_party/gocheck
