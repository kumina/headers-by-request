package headers_by_request

// Config the plugin configuration.
type Config struct {
	UrlHeaderRequest string `json:"urlHeaderRequest,omitempty"`
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{ UrlHeaderRequest: ""}
}
