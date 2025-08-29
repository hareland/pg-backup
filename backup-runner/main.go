package main

import (
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
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
	Endpoint string `yaml:"endpoint"`
	Access   string `yaml:"accessKey"`
	Secret   string `yaml:"secretKey"`
	Region   string `yaml:"region"`
}

type Backup struct {
	URL         string `yaml:"url"`
	Destination string `yaml:"destination"`
	Schedule    string `yaml:"schedule"`
	MaxHistory  int    `yaml:"maxHistory"`
}

type s3Object struct {
	Key          string    `json:"Key"`
	LastModified time.Time `json:"LastModified"`
	Size         int64     `json:"Size"`
	ETag         string    `json:"ETag"`
	StorageClass string    `json:"StorageClass"`
}

/*
   Expand env across the entire YAML before parsing.
   Supports:
     - $VAR
     - ${VAR}
     - ${VAR:-default}   (use default if VAR is unset or empty)
     - ${VAR-default}    (use default if VAR is unset)
*/
var envPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?::(-)?([^}]*))?\}|\$([A-Za-z_][A-Za-z0-9_]*)`)

func expandAllEnv(s string) string {
	return envPattern.ReplaceAllStringFunc(s, func(m string) string {
		sub := envPattern.FindStringSubmatch(m)
		// Groups:
		// 1 = var in ${...}, 2 = "-" if ":-" else "", 3 = default (maybe empty), 4 = var in $VAR
		varName := sub[1]
		if varName == "" {
			varName = sub[4]
		}
		if varName == "" {
			return m
		}
		val, ok := os.LookupEnv(varName)

		// Default handling
		if sub[1] != "" { // ${...} form may include default
			def := sub[3]
			hasColonDash := sub[2] == "-" // true means ":-" (unset OR empty)
			if def != "" {
				if hasColonDash {
					if !ok || val == "" {
						return def
					}
				} else {
					if !ok {
						return def
					}
				}
			}
		}

		if !ok {
			return "" // unset & no default => empty
		}
		return val
	})
}

func fillDestFromEnv(d *Destination) {
	// Values are already expanded; these are fallbacks if still empty.
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

func awsEnv(endpoint, region, access, secret string) []string {
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
	return env
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
	cmd.Env = awsEnv(endpoint, region, access, secret)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func awsListObjects(endpoint, region, access, secret, bucket, prefix string) ([]s3Object, error) {
	args := []string{
		"s3api", "list-objects-v2",
		"--bucket", bucket,
		"--prefix", strings.TrimLeft(prefix, "/"),
		"--output", "json",
	}
	if endpoint != "" {
		args = append(args, "--endpoint-url", endpoint)
	}
	if region != "" {
		args = append(args, "--region", region)
	}
	cmd := exec.Command("aws", args...)
	cmd.Env = awsEnv(endpoint, region, access, secret)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var payload struct {
		Contents []s3Object `json:"Contents"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return nil, err
	}
	return payload.Contents, nil
}

func awsDeleteObjects(endpoint, region, access, secret, bucket string, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	for start := 0; start < len(keys); start += 1000 {
		end := start + 1000
		if end > len(keys) {
			end = len(keys)
		}
		batch := keys[start:end]

		type delObj struct {
			Key string `json:"Key"`
		}
		body, _ := json.Marshal(struct {
			Objects []delObj `json:"Objects"`
			Quiet   bool     `json:"Quiet"`
		}{
			Objects: func() []delObj {
				out := make([]delObj, len(batch))
				for i, k := range batch {
					out[i] = delObj{Key: k}
				}
				return out
			}(),
			Quiet: true,
		})

		args := []string{
			"s3api", "delete-objects",
			"--bucket", bucket,
			"--delete", string(body),
		}
		if endpoint != "" {
			args = append(args, "--endpoint-url", endpoint)
		}
		if region != "" {
			args = append(args, "--region", region)
		}
		cmd := exec.Command("aws", args...)
		cmd.Env = awsEnv(endpoint, region, access, secret)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	return nil
}

func pruneHistory(dest Destination, basePrefix string, keep int) {
	if keep <= 0 {
		return
	}
	objs, err := awsListObjects(dest.Endpoint, dest.Region, dest.Access, dest.Secret, dest.Bucket, basePrefix)
	if err != nil {
		log.Printf("[prune] list failed for s3://%s/%s: %v", dest.Bucket, basePrefix, err)
		return
	}

	filtered := make([]s3Object, 0, len(objs))
	for _, o := range objs {
		if strings.HasSuffix(o.Key, ".dump") && strings.HasPrefix(filepath.Base(o.Key), "pgdump-") {
			filtered = append(filtered, o)
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].LastModified.After(filtered[j].LastModified)
	})

	if len(filtered) <= keep {
		return
	}

	toDelete := make([]string, 0, len(filtered)-keep)
	for _, o := range filtered[keep:] {
		toDelete = append(toDelete, o.Key)
	}
	log.Printf("[prune] deleting %d old backups under s3://%s/%s", len(toDelete), dest.Bucket, basePrefix)
	if err := awsDeleteObjects(dest.Endpoint, dest.Region, dest.Access, dest.Secret, dest.Bucket, toDelete); err != nil {
		log.Printf("[prune] delete failed: %v", err)
	}
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

	// Expand env across the entire YAML so all fields support env vars.
	expanded := expandAllEnv(string(raw))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		log.Fatalf("parse config: %v", err)
	}

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

			dbname := "all"
			if i := strings.LastIndex(b.URL, "/"); i >= 0 && i < len(b.URL)-1 {
				dbname = b.URL[i+1:]
				if strings.Contains(dbname, "?") {
					dbname = strings.SplitN(dbname, "?", 2)[0]
				}
			}
			basePrefix := filepath.Join(strings.Trim(dest.Prefix, "/"), dbname) + "/"

			ts := time.Now().UTC().Format("20060102T150405Z")
			key := basePrefix + "pgdump-" + ts + ".dump"

			if err := awsCp(dest.Endpoint, dest.Region, dest.Access, dest.Secret, dest.Bucket, key, out); err != nil {
				log.Printf("[backup] upload failed: %v", err)
				return
			}
			log.Printf("[backup] uploaded s3://%s/%s", dest.Bucket, key)

			if _, err := os.Stat("/backups"); err == nil {
				_ = os.Rename(out, filepath.Join("/backups", filepath.Base(out)))
			}

			if b.MaxHistory > 0 {
				pruneHistory(dest, basePrefix, b.MaxHistory)
			}
		})
		if err != nil {
			log.Fatalf("schedule %q: %v", b.Schedule, err)
		}
	}

	log.Printf("scheduler running…")
	c.Run()
}
