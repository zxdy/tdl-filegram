package bootstrap

import (
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 集中配置，启动时从 YAML 加载并校验，随后用环境变量覆盖
type Config struct {
	App      AppConfig      `yaml:"app"`
	Log      LogConfig      `yaml:"log"`
	Database DatabaseConfig `yaml:"database"`
	Download DownloadConfig `yaml:"download"`
	Telegram TelegramConfig `yaml:"telegram"`
	Bot      BotConfig      `yaml:"bot"`
}

type AppConfig struct {
	Name string `yaml:"name"`
	Host string `yaml:"host"`
	Port string `yaml:"port"`
	Env  string `yaml:"env"`
}

type LogConfig struct {
	Level string `yaml:"level"`
	Dir   string `yaml:"dir"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type DownloadConfig struct {
	Dir     string `yaml:"dir"`
	Threads int    `yaml:"threads"`
	Limit   int    `yaml:"limit"`
}

type TelegramConfig struct {
	AppID            int    `yaml:"app_id"`
	AppHash          string `yaml:"app_hash"`
	DataDir          string `yaml:"data_dir"`
	Namespace        string `yaml:"namespace"`
	PoolSize         int    `yaml:"pool_size"`
	ReconnectTimeout string `yaml:"reconnect_timeout"`
	Proxy            string `yaml:"proxy"`
}

// BotConfig 是 Telegram Bot API 入口配置。允许用户为空时 Bot 不启动。
type BotConfig struct {
	Token          string  `yaml:"token"`
	AllowedUserIDs []int64 `yaml:"allowed_user_ids"`
}

// LoadConfig 加载配置文件（文件缺失时用默认值），再用环境变量覆盖。
// 支持的环境变量见 applyEnvOverrides。
func LoadConfig(path string) (*Config, error) {
	var cfg Config
	if b, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(b, &cfg); err != nil {
			return nil, err
		}
	}
	cfg.applyDefaults()
	cfg.applyEnvOverrides()
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.App.Host == "" {
		c.App.Host = "0.0.0.0"
	}
	if c.App.Port == "" {
		c.App.Port = "8743"
	}
	if c.App.Env == "" {
		c.App.Env = "debug"
	}
	if c.Log.Level == "" {
		c.Log.Level = "info"
	}
	if c.Log.Dir == "" {
		c.Log.Dir = "logs"
	}
	if c.Database.Path == "" {
		c.Database.Path = "data/tdl-filegram.db"
	}
	if c.Download.Dir == "" {
		c.Download.Dir = "downloads"
	}
	if c.Download.Threads == 0 {
		c.Download.Threads = 4
	}
	if c.Download.Limit == 0 {
		c.Download.Limit = 2
	}
	if c.Telegram.DataDir == "" {
		c.Telegram.DataDir = ".tdl"
	}
	if c.Telegram.Namespace == "" {
		c.Telegram.Namespace = "default"
	}
	if c.Telegram.PoolSize == 0 {
		c.Telegram.PoolSize = 8
	}
	if c.Telegram.ReconnectTimeout == "" {
		c.Telegram.ReconnectTimeout = "5m"
	}
}

// applyEnvOverrides 用环境变量覆盖配置，空字符串/解析失败时保留原值。
// Docker 运行时通过这些环境变量配置各选项。
func (c *Config) applyEnvOverrides() {
	if v := os.Getenv("APP_HOST"); v != "" {
		c.App.Host = v
	}
	if v := os.Getenv("APP_PORT"); v != "" {
		c.App.Port = v
	}
	if v := os.Getenv("DB_PATH"); v != "" {
		c.Database.Path = v
	}
	if v := os.Getenv("DOWNLOAD_DIR"); v != "" {
		c.Download.Dir = v
	}
	if v := os.Getenv("DOWNLOAD_THREADS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Download.Threads = n
		}
	}
	if v := os.Getenv("DOWNLOAD_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Download.Limit = n
		}
	}
	if v := os.Getenv("TG_APP_ID"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Telegram.AppID = n
		}
	}
	if v := os.Getenv("TG_APP_HASH"); v != "" {
		c.Telegram.AppHash = v
	}
	if v := os.Getenv("TG_DATA_DIR"); v != "" {
		c.Telegram.DataDir = v
	}
	if v := os.Getenv("TG_NAMESPACE"); v != "" {
		c.Telegram.Namespace = v
	}
	if v := os.Getenv("TG_POOL_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Telegram.PoolSize = n
		}
	}
	if v := os.Getenv("TG_RECONNECT_TIMEOUT"); v != "" {
		c.Telegram.ReconnectTimeout = v
	}
	if v := os.Getenv("TG_PROXY"); v != "" {
		c.Telegram.Proxy = v
	}
	if v := os.Getenv("BOT_TOKEN"); v != "" {
		c.Bot.Token = v
	}
	if v := os.Getenv("BOT_ALLOWED_USER_IDS"); v != "" {
		ids := make([]int64, 0)
		for _, part := range strings.Split(v, ",") {
			if id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); err == nil {
				ids = append(ids, id)
			}
		}
		c.Bot.AllowedUserIDs = ids
	}
}

// ParseReconnectTimeout 解析重连超时
func (c *TelegramConfig) ParseReconnectTimeout() time.Duration {
	d, err := time.ParseDuration(c.ReconnectTimeout)
	if err != nil {
		return 5 * time.Minute
	}
	return d
}
