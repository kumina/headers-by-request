package headers_by_request

// Config the plugin configuration.
type Config struct {
	UrlHeaderRequest string `json:"urlHeaderRequest,omitempty"`
	EnableTiming bool `json:"enableTiming,omitempty"`
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{
		UrlHeaderRequest: "",
		EnableTiming: false}
}
