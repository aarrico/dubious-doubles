package config

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	HTTPAddr        string
	DatabaseURL     string
	DatabaseTimeout time.Duration
}

func Load(getenv func(string) string) (Config, error) {
	c := Config{HTTPAddr: ":8080", DatabaseURL: getenv("DATABASE_URL"), DatabaseTimeout: 2 * time.Second}
	if s := getenv("HTTP_ADDR"); s != "" {
		c.HTTPAddr = s
	}
	_, port, err := net.SplitHostPort(c.HTTPAddr)
	n, portErr := strconv.Atoi(port)
	if err != nil || portErr != nil || n < 1 || n > 65535 {
		return Config{}, errors.New("HTTP_ADDR must contain a host and port between 1 and 65535")
	}
	u, err := url.Parse(c.DatabaseURL)
	if err != nil || u == nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Hostname() == "" || u.Path == "" || u.Path == "/" {
		return Config{}, errors.New("DATABASE_URL must be a PostgreSQL URL with a host and database")
	}
	if _, err := pgxpool.ParseConfig(c.DatabaseURL); err != nil {
		return Config{}, errors.New("DATABASE_URL is invalid")
	}
	if s := getenv("DATABASE_TIMEOUT"); s != "" {
		c.DatabaseTimeout, err = time.ParseDuration(s)
		if err != nil || c.DatabaseTimeout < time.Millisecond || c.DatabaseTimeout > 5*time.Second {
			return Config{}, errors.New("DATABASE_TIMEOUT must be between 1ms and 5s")
		}
	}
	return c, nil
}
