package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Destinations map[string]Destination `yaml:"destinations"`
	Backups      []Backup               `yaml:"backups"`
}

type Destination struct {
	Bucket   string `yaml:"bucket"`
	Prefix   string `yaml:"prefix"`
	Endpoint string `yaml:"endpoint"` // e.g. http://minio:9000 or https://<r2acct>.r2.cloudflarestorage.com
	Access   string `yaml:"accessKey"`
	Secret   string `yaml:"secretKey"`
	Region   string `yaml:"region"`
}

type Backup struct {
	URL         string `yaml:"url"`
	Destination string `yaml:"destination"`
	Schedule    string `yaml:"schedule"`
}

// expand ${VAR} style strings
func resolveEnv(s string) string {
	return os.ExpandEnv(s)
}

// fill missing fields from env
func fillDestFromEnv(d *Destination) {
	if v := resolveEnv(d.Access); v != "" {
		d.Access = v
	}
	if v := resolveEnv(d.Secret); v != "" {
		d.Secret = v
	}
	if v := resolveEnv(d.Endpoint); v != "" {
		d.Endpoint = v
	}
	if v := resolveEnv(d.Region); v != "" {
		d.Region = v
	}

	if d.Access == "" {
		d.Access = os.Getenv("AWS_ACCESS_KEY_ID")
	}
	if d.Secret == "" {
		d.Secret = os.Getenv("AWS_SECRET_ACCESS_KEY")
	}
	if d.Region == "" {
		d.Region = os.Getenv("AWS_DEFAULT_REGION")
	}
	if d.Endpoint == "" {
		d.Endpoint = os.Getenv("AWS_ENDPOINT_URL")
	}
}

func runPgDump(url string) (string, error) {
	ts := time.Now().UTC().Format("20060102T150405Z")
	out := filepath.Join("/tmp", "pgdump-"+ts+".dump")
	cmd := exec.Command("pg_dump", "-Fc", url, "-f", out)
	cmd.Env = append(os.Environ(), "PGCONNECT_TIMEOUT=10")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return out, cmd.Run()
}

func awsCp(endpoint, region, access, secret, bucket, key, file string) error {
	args := []string{"s3", "cp", file, "s3://" + bucket + "/" + strings.TrimLeft(key, "/")}
	if endpoint != "" {
		args = append(args, "--endpoint-url", endpoint)
	}
	if region != "" {
		args = append(args, "--region", region)
	}
	cmd := exec.Command("aws", args...)
	env := os.Environ()
	if access != "" {
		env = append(env, "AWS_ACCESS_KEY_ID="+access)
	}
	if secret != "" {
		env = append(env, "AWS_SECRET_ACCESS_KEY="+secret)
	}
	if region != "" {
		env = append(env, "AWS_DEFAULT_REGION="+region)
	}
	if endpoint != "" {
		env = append(env, "AWS_ENDPOINT_URL="+endpoint)
	}
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func main() {
	cfgFile := os.Getenv("CONFIG_FILE")
	if cfgFile == "" {
		cfgFile = "/config.yaml"
	}
	raw, err := os.ReadFile(cfgFile)
	if err != nil {
		log.Fatalf("read config: %v", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		log.Fatalf("parse config: %v", err)
	}

	// resolve env placeholders and fallbacks
	for k, d := range cfg.Destinations {
		fillDestFromEnv(&d)
		cfg.Destinations[k] = d
	}

	parser := cron.NewParser(cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	c := cron.New(cron.WithParser(parser), cron.WithChain(cron.Recover(cron.DefaultLogger)))

	for _, b := range cfg.Backups {
		b := b
		dest, ok := cfg.Destinations[b.Destination]
		if !ok {
			log.Fatalf("unknown destination %q", b.Destination)
		}
		_, err := c.AddFunc(b.Schedule, func() {
			log.Printf("[backup] start %s", b.URL)
			out, err := runPgDump(b.URL)
			if err != nil {
				log.Printf("[backup] pg_dump failed: %v", err)
				return
			}
			defer os.Remove(out)

			ts := time.Now().UTC().Format("20060102T150405Z")
			dbname := "all"
			if i := strings.LastIndex(b.URL, "/"); i >= 0 && i < len(b.URL)-1 {
				dbname = b.URL[i+1:]
				if strings.Contains(dbname, "?") {
					dbname = strings.SplitN(dbname, "?", 2)[0]
				}
			}
			key := filepath.Join(strings.Trim(dest.Prefix, "/"), dbname, "pgdump-"+ts+".dump")

			if err := awsCp(dest.Endpoint, dest.Region, dest.Access, dest.Secret, dest.Bucket, key, out); err != nil {
				log.Printf("[backup] upload failed: %v", err)
				return
			}
			log.Printf("[backup] uploaded s3://%s/%s", dest.Bucket, key)

			if _, err := os.Stat("/backups"); err == nil {
				_ = os.Rename(out, filepath.Join("/backups", filepath.Base(out)))
			}
		})
		if err != nil {
			log.Fatalf("schedule %q: %v", b.Schedule, err)
		}
	}

	log.Printf("scheduler running…")
	c.Run()
}
