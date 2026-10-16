package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	c, err := Load(func(key string) string {
		if key == "DATABASE_URL" {
			return "postgres://user:secret@db/app"
		}
		return ""
	})
	if err != nil || c.HTTPAddr != ":8080" || c.DatabaseTimeout != 2*time.Second {
		t.Fatalf("unexpected configuration: %+v, %v", c, err)
	}
}

func TestInvalidConfiguration(t *testing.T) {
	for _, tc := range []struct{ name, key, value string }{
		{"missing database", "DATABASE_URL", ""},
		{"wrong scheme", "DATABASE_URL", "https://user:secret@db/app"},
		{"missing database name", "DATABASE_URL", "postgres://user:secret@db"},
		{"invalid database option", "DATABASE_URL", "postgres://user:secret@db/app?pool_max_conns=no"},
		{"invalid address", "HTTP_ADDR", "localhost"},
		{"invalid port", "HTTP_ADDR", ":0"},
		{"invalid timeout", "DATABASE_TIMEOUT", "no"},
		{"unbounded timeout", "DATABASE_TIMEOUT", "1h"},
		{"negative timeout", "DATABASE_TIMEOUT", "-1s"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(func(key string) string {
				if key == tc.key {
					return tc.value
				}
				if key == "DATABASE_URL" {
					return "postgres://user:secret@db/app"
				}
				return ""
			})
			if err == nil {
				t.Fatal("expected configuration failure")
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatal("error exposed credentials")
			}
		})
	}
}
