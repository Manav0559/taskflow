// Package config loads process configuration from the environment. All services
// (api, scheduler, worker) share this loader so deployment (docker-compose/k8s) only
// has one set of env vars to reason about.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	DatabaseURL    string
	HTTPAddr       string
	JWTSecret      string
	LeaseDuration  time.Duration
	PollInterval   time.Duration
	WorkerID       string
	MetricsAddr    string
	RateLimitRPS   float64
	RateLimitBurst int
	Concurrency    int
	OTLPEndpoint   string
	RedisAddr      string
	ReplicaURL     string
}

func Load() (Config, error) {
	cfg := Config{
		DatabaseURL:  getEnv("DATABASE_URL", "postgres://taskflow:taskflow@localhost:5432/taskflow?sslmode=disable"),
		HTTPAddr:     getEnv("HTTP_ADDR", ":8080"),
		JWTSecret:    getEnv("JWT_SECRET", ""),
		MetricsAddr:  getEnv("METRICS_ADDR", ":9090"),
		WorkerID:     getEnv("WORKER_ID", ""),
		OTLPEndpoint: getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		RedisAddr:    getEnv("REDIS_ADDR", ""),
		ReplicaURL:   getEnv("REPLICA_DATABASE_URL", ""),
	}

	var err error
	if cfg.LeaseDuration, err = getEnvDuration("LEASE_DURATION", 30*time.Second); err != nil {
		return cfg, err
	}
	if cfg.PollInterval, err = getEnvDuration("POLL_INTERVAL", 1*time.Second); err != nil {
		return cfg, err
	}
	if cfg.RateLimitRPS, err = getEnvFloat("RATE_LIMIT_RPS", 20); err != nil {
		return cfg, err
	}
	if cfg.RateLimitBurst, err = getEnvInt("RATE_LIMIT_BURST", 40); err != nil {
		return cfg, err
	}
	if cfg.Concurrency, err = getEnvInt("WORKER_CONCURRENCY", 4); err != nil {
		return cfg, err
	}

	if cfg.WorkerID == "" {
		host, _ := os.Hostname()
		cfg.WorkerID = fmt.Sprintf("%s-%d", host, os.Getpid())
	}

	return cfg, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvDuration(key string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	return time.ParseDuration(v)
}

func getEnvFloat(key string, def float64) (float64, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	return strconv.ParseFloat(v, 64)
}

func getEnvInt(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	return strconv.Atoi(v)
}
