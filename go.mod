module github.com/gomatic/yze

go 1.26.4

require (
	github.com/gomatic/go-error v0.3.0
	github.com/gomatic/go-yze v0.0.0
	github.com/gomatic/yze-go-errconst v0.0.0
	github.com/gomatic/yze-go-gotostmt v0.0.0
	github.com/gomatic/yze-go-namedtypes v0.0.0
	github.com/stretchr/testify v1.11.1
	github.com/urfave/cli/v3 v3.10.1
	golang.org/x/tools v0.47.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/mod v0.37.0 // indirect
	golang.org/x/sync v0.21.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// These modules are not yet published; resolve them locally until they are tagged.
replace (
	github.com/gomatic/go-yze => ../go-yze
	github.com/gomatic/yze-go-errconst => ../yze-go-errconst
	github.com/gomatic/yze-go-gotostmt => ../yze-go-gotostmt
	github.com/gomatic/yze-go-namedtypes => ../yze-go-namedtypes
)
