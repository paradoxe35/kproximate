package config

import (
	"context"
	"regexp"

	"github.com/sethvargo/go-envconfig"
)

type KproximateConfig struct {
	Debug                   bool   `env:"debug"`
	KpJoinCommand           string `env:"kpJoinCommand"`
	KpNodeCores             int    `env:"kpNodeCores"`
	KpNodeDisableSsh        bool   `env:"kpNodeDisableSsh"`
	KpNodeMemory            int    `env:"kpNodeMemory"`
	KpNodeLabels            string `env:"kpNodeLabels"`
	KpNodeNamePrefix        string `env:"kpNodeNamePrefix"`
	KpNodeNameRegex         regexp.Regexp
	KpNodeParams            map[string]interface{}
	KpNodeTemplateName      string  `env:"kpNodeTemplateName"`
	KpQemuExecJoin          bool    `env:"kpQemuExecJoin"`
	KpLocalTemplateStorage  bool    `env:"kpLocalTemplateStorage"`
	LoadHeadroom            float64 `env:"loadHeadroom"`
	MaxKpNodes              int     `env:"maxKpNodes"`
	PmAllowInsecure         bool    `env:"pmAllowInsecure"`
	PmDebug                 bool    `env:"pmDebug"`
	PmPassword              string  `env:"pmPassword"`
	PmToken                 string  `env:"pmToken"`
	PmUrl                   string  `env:"pmUrl"`
	PmUserID                string  `env:"pmUserID"`
	PollInterval            int     `env:"pollInterval"`
	SshKey                  string  `env:"sshKey"`
	WaitSecondsForJoin      int     `env:"waitSecondsForJoin"`
	WaitSecondsForProvision int     `env:"waitSecondsForProvision"`

	// Node Selection Strategy Configuration
	NodeSelectionStrategy string `env:"nodeSelectionStrategy,default=spread"`
	MinAvailableCpuCores  int    `env:"minAvailableCpuCores,default=0"`
	MinAvailableMemoryMB  int    `env:"minAvailableMemoryMB,default=0"`
	ExcludedNodes         string `env:"excludedNodes"`
}

type RabbitConfig struct {
	Host     string `env:"rabbitMQHost"`
	Password string `env:"rabbitMQPassword"`
	Port     int    `env:"rabbitMQPort"`
	User     string `env:"rabbitMQUser"`
	UseTLS   bool   `env:"rabbitMQUseTLS,default=true"`
}

func GetKpConfig() (KproximateConfig, error) {
	config := &KproximateConfig{}

	err := envconfig.Process(context.Background(), config)
	if err != nil {
		return *config, err
	}

	*config = validateConfig(config)

	return *config, nil
}

func GetRabbitConfig() (RabbitConfig, error) {
	config := &RabbitConfig{}

	err := envconfig.Process(context.Background(), config)
	if err != nil {
		return *config, err
	}

	return *config, nil
}

func validateConfig(config *KproximateConfig) KproximateConfig {
	if config.LoadHeadroom < 0.2 {
		config.LoadHeadroom = 0.2
	}

	if config.PollInterval < 10 {
		config.PollInterval = 10
	}

	if config.WaitSecondsForJoin < 60 {
		config.WaitSecondsForJoin = 60
	}

	if config.WaitSecondsForProvision < 60 {
		config.WaitSecondsForProvision = 60
	}

	// replace all \n with actual newlines
	config.KpJoinCommand = regexp.MustCompile(`\\n`).ReplaceAllString(config.KpJoinCommand, "\n")

	// Validate node selection strategy
	validStrategies := map[string]bool{
		"spread":      true,
		"max-memory":  true,
		"max-cpu":     true,
		"balanced":    true,
		"round-robin": true,
	}
	if !validStrategies[config.NodeSelectionStrategy] {
		config.NodeSelectionStrategy = "spread"
	}

	// Ensure minimum resource requirements are non-negative
	if config.MinAvailableCpuCores < 0 {
		config.MinAvailableCpuCores = 0
	}

	if config.MinAvailableMemoryMB < 0 {
		config.MinAvailableMemoryMB = 0
	}

	return *config
}
