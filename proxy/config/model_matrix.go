package config

type ModelSourceConfig struct {
	Path             string         `yaml:"path"`
	Cmd              string         `yaml:"cmd"`
	CmdStop          string         `yaml:"cmdStop"`
	Proxy            string         `yaml:"proxy"`
	Aliases          []string       `yaml:"aliases"`
	Env              []string       `yaml:"env"`
	CheckEndpoint    string         `yaml:"checkEndpoint"`
	UnloadAfter      int            `yaml:"ttl"`
	Unlisted         bool           `yaml:"unlisted"`
	UseModelName     string         `yaml:"useModelName"`
	VramMB           int            `yaml:"vramMb"`
	MinVramMB        int            `yaml:"minVramMb"`
	FitPolicy        string         `yaml:"fitPolicy"`
	CpuMoe           int            `yaml:"cpuMoe"`
	Name             string         `yaml:"name"`
	Description      string         `yaml:"description"`
	ConcurrencyLimit int            `yaml:"concurrencyLimit"`
	Filters          ModelFilters   `yaml:"filters"`
	Macros           MacroList      `yaml:"macros"`
	Metadata         map[string]any `yaml:"metadata"`
	SendLoadingState *bool          `yaml:"sendLoadingState"`
}

type ParameterSetConfig struct {
	Args             string         `yaml:"args"`
	Aliases          []string       `yaml:"aliases"`
	Env              []string       `yaml:"env"`
	UnloadAfter      int            `yaml:"ttl"`
	Unlisted         bool           `yaml:"unlisted"`
	UseModelName     string         `yaml:"useModelName"`
	VramMB           int            `yaml:"vramMb"`
	MinVramMB        int            `yaml:"minVramMb"`
	FitPolicy        string         `yaml:"fitPolicy"`
	CpuMoe           int            `yaml:"cpuMoe"`
	Name             string         `yaml:"name"`
	Description      string         `yaml:"description"`
	ConcurrencyLimit int            `yaml:"concurrencyLimit"`
	Filters          ModelFilters   `yaml:"filters"`
	Macros           MacroList      `yaml:"macros"`
	Metadata         map[string]any `yaml:"metadata"`
	SendLoadingState *bool          `yaml:"sendLoadingState"`
}
