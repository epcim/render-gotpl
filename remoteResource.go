package main

var DefaultResource = RemoteResource{
	Name:         "",
	Repo:         "",
	RepoCreds:    "",
	Update:       false,
	Template:     "",
	TemplateGlob: "*.t*pl",
	Kinds:        []string{"!namespace"},
	FlattenValuesBy: "_",
	destDir:         "",
}

// RemoteResource is specification for remote templates (git, s3, http...)
type RemoteResource struct {
	// local name for remote
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
	// go-getter compatible uri to remote
	Repo string `json:"repo" yaml:"repo"`
	// go-getter creds profile for private repos, s3, etc..
	RepoCreds string `json:"repoCreds" yaml:"repoCreds"`
	// whether to update existing source
	Update bool `json:"update,omitempty" yaml:"update,omitempty"`
	// template
	Template     string `json:"template,omitempty" yaml:"template,omitempty"`
	TemplateGlob string `json:"templateGlob,omitempty" yaml:"templateGlob,omitempty"`

	// kinds
	Kinds []string `json:"kinds,omitempty" yaml:"kinds,omitempty"`

	//Values are flattened to single level, default: `_` delimited: .server_port: 111
	FlattenValuesBy string `json:"flattenValuesBy,omitempty" yaml:"flattenValuesBy,omitempty"`

	// destDir is where the resource is cloned
	destDir string
}
