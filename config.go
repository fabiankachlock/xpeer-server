package xpeer

import (
	"github.com/spf13/viper"
)

const (
	DEFAULT_HOST = "0.0.0.0"
	DEFAULT_PORT = "8102"
)

// read config from xpeer.env
func getConfig() ServerConfig {
	// configure viper to read xpeer.env
	viper.SetConfigName("xpeer")
	viper.AddConfigPath(".")
	viper.AutomaticEnv()
	viper.SetConfigType("env")

	// set default values
	viper.SetDefault("XPEER_HOST", DEFAULT_HOST)
	viper.SetDefault("XPEER_PORT", DEFAULT_PORT)

	// read config file
	if err := viper.ReadInConfig(); err != nil {
		logWarn.Printf("error reading config: %s", err)
	}

	// read configured values
	host, hostOk := viper.Get("XPEER_HOST").(string)
	port, portOk := viper.Get("XPEER_PORT").(string)
	if !hostOk || !portOk {
		logError.Fatalf("Invalid type assertion")
	}

	// return config
	return ServerConfig{
		Host:           host,
		Port:           port,
		VerboseLogging: true,
	}
}
