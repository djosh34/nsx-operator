package nostructerrorreturns

type Config struct {
	Name string
}

func BuildConfig() (Config, error) { // want "functions returning a struct and error must return \\*Struct, error"
	return Config{}, nil
}

func BuildConfigPointer() (*Config, error) {
	return &Config{}, nil
}

type Code string

func BuildCode() (Code, error) {
	return Code("ready"), nil
}

//projectlint:allow struct-error-return meaningful zero-value fixture
func BuildAllowedConfig() (Config, error) {
	return Config{}, nil
}
