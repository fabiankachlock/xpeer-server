package util

import (
	"github.com/fabiankachlock/xpeer-server/pkg/xpeer"
	"github.com/spf13/viper"
)

// util

// read config from xpeer.env
func GetConfig() xpeer.ServerConfig {
	// configure viper to read xpeer.env
	viper.SetConfigName("xpeer")
	viper.AddConfigPath(".")
	viper.AutomaticEnv()
	viper.SetConfigType("env")

	// set default values
	viper.SetDefault("XPEER_HOST", "0.0.0.0")
	viper.SetDefault("XPEER_PORT", "8192")

	// read config file
	if err := viper.ReadInConfig(); err != nil {
		xpeer.WarnLogger.Printf("error reading config: %s", err)
	}

	// read configured values
	host, hostOk := viper.Get("XPEER_HOST").(string)
	port, portOk := viper.Get("XPEER_PORT").(string)
	if !hostOk || !portOk {
		xpeer.ErrorLogger.Fatalf("Invalid type assertion")
	}

	// return config
	return xpeer.ServerConfig{
		Host:           host,
		Port:           port,
		VerboseLogging: true,
	}
}

// return given slice without specified string (id)
