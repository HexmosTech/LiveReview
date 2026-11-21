package cmd

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/livereview/internal/blobstore"
	"github.com/livereview/internal/database"
	"github.com/urfave/cli/v2"
	"golang.org/x/term"
)

const blobStorageRowName = "blob_storage"

// StorageCommand groups instance-wide storage maintenance commands.
func StorageCommand() *cli.Command {
	return &cli.Command{
		Name:  "storage",
		Usage: "Manage the instance-wide blob storage settings",
		Subcommands: []*cli.Command{
			{
				Name:  "ensure",
				Usage: "Verify the blob_storage row exists and is usable; prompt to insert it when missing",
				Description: "Reads system_settings.blob_storage from the database selected by --env-file.\n" +
					"If the row exists and is valid, this is a read-only check (plus an optional\n" +
					"write/read/delete marker test with --test). If the row is missing or invalid,\n" +
					"it prompts for the values and writes them - but only when stdin is a terminal.",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "env-file",
						Usage: "Path to the .env file that provides DATABASE_URL (e.g. .env.prod)",
					},
					&cli.BoolFlag{
						Name:  "test",
						Usage: "After ensuring, round-trip a marker object through the bucket",
					},
					&cli.BoolFlag{
						Name:  "no-interactive",
						Usage: "Never prompt; fail if the row is missing or invalid",
					},
				},
				Action: runStorageEnsure,
			},
		},
	}
}

func runStorageEnsure(c *cli.Context) error {
	if envFile := c.String("env-file"); envFile != "" {
		if err := LoadEnvFile(envFile); err != nil {
			return fmt.Errorf("failed to load env file %s: %w", envFile, err)
		}
	}
	if strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		return errors.New("DATABASE_URL is not set; pass --env-file (e.g. --env-file .env.prod)")
	}

	db, err := database.NewDB()
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer db.Close()

	printDBTarget()

	cfg, found, err := loadBlobStorageConfig(db)
	if err != nil {
		return err
	}

	interactive := !c.Bool("no-interactive") && term.IsTerminal(int(os.Stdin.Fd()))

	if found {
		if err := blobstore.ValidateConfig(&cfg); err == nil {
			fmt.Println("✅ blob_storage row exists and is valid.")
			printConfigSummary(cfg)
			return runStorageTest(c, cfg)
		} else {
			fmt.Printf("⚠️  blob_storage row exists but is invalid: %v\n", err)
		}
	} else {
		fmt.Println("⚠️  No blob_storage row exists in system_settings.")
	}

	if !interactive {
		return errors.New("blob_storage is missing or invalid and stdin is not a terminal; " +
			"re-run without --no-interactive, or configure it once in Settings → Storage")
	}

	updated, err := promptStorageConfig(cfg)
	if err != nil {
		return err
	}
	if err := blobstore.ValidateConfig(&updated); err != nil {
		return fmt.Errorf("the values entered are not valid: %w", err)
	}

	fmt.Println("\nAbout to write these values to system_settings.blob_storage:")
	printConfigSummary(updated)
	fmt.Print("Write these settings? Type 'yes' to continue: ")
	answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	if strings.TrimSpace(strings.ToLower(answer)) != "yes" {
		return errors.New("aborted; nothing was written")
	}

	if err := saveBlobStorageConfig(db, updated); err != nil {
		return err
	}
	fmt.Println("✅ blob_storage settings saved.")

	return runStorageTest(c, updated)
}

func loadBlobStorageConfig(db *sql.DB) (blobstore.Config, bool, error) {
	var raw []byte
	err := db.QueryRow(`SELECT data FROM system_settings WHERE name = $1`, blobStorageRowName).Scan(&raw)
	if err == sql.ErrNoRows {
		return blobstore.Config{}, false, nil
	}
	if err != nil {
		return blobstore.Config{}, false, fmt.Errorf("read %s settings: %w", blobStorageRowName, err)
	}

	var cfg blobstore.Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return blobstore.Config{}, false, fmt.Errorf("parse %s settings: %w", blobStorageRowName, err)
	}
	return cfg, true, nil
}

func saveBlobStorageConfig(db *sql.DB, cfg blobstore.Config) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal storage settings: %w", err)
	}
	_, err = db.Exec(`
		INSERT INTO system_settings (name, data)
		VALUES ($1, $2)
		ON CONFLICT (name) DO UPDATE SET data = EXCLUDED.data, updated_at = CURRENT_TIMESTAMP
	`, blobStorageRowName, data)
	if err != nil {
		return fmt.Errorf("save storage settings: %w", err)
	}
	return nil
}

func runStorageTest(c *cli.Context, cfg blobstore.Config) error {
	if !c.Bool("test") {
		return nil
	}
	if cfg.Backend == blobstore.BackendFilesystem {
		fmt.Println("ℹ️  Filesystem backend selected; skipping the connectivity test (it would write to the deploy machine, not the server).")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	fmt.Print("Testing storage connection (write/read/delete)... ")
	if err := blobstore.CheckWritable(ctx, cfg); err != nil {
		return fmt.Errorf("storage connectivity test failed: %w", err)
	}
	fmt.Println("ok")
	return nil
}

func promptStorageConfig(current blobstore.Config) (blobstore.Config, error) {
	reader := bufio.NewReader(os.Stdin)
	cfg := current

	defBackend := current.Backend
	if defBackend == "" {
		defBackend = blobstore.BackendS3
	}
	backend := strings.ToLower(promptLine(reader, "Backend (s3 or filesystem)", defBackend))

	switch backend {
	case blobstore.BackendS3:
		cfg.Backend = blobstore.BackendS3
		cfg.Bucket = promptLine(reader, "Bucket", current.Bucket)
		cfg.Endpoint = promptLine(reader, "Endpoint (optional, e.g. https://s3.us-east-005.backblazeb2.com)", current.Endpoint)
		region := current.Region
		if region == "" {
			region = "us-east-1"
		}
		cfg.Region = promptLine(reader, "Region", region)
		cfg.AccessKeyID = promptLine(reader, "Access key ID", current.AccessKeyID)

		secret, err := promptSecret("Secret access key (leave blank to keep the current value)")
		if err != nil {
			return cfg, fmt.Errorf("read secret: %w", err)
		}
		if secret != "" {
			cfg.SecretAccessKey = secret
		}
		if cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
			fmt.Println("⚠️  No static credentials entered; the AWS SDK default credential chain (instance role, etc.) will be used.")
		}
	case blobstore.BackendFilesystem:
		cfg.Backend = blobstore.BackendFilesystem
		localDir := current.LocalDir
		if localDir == "" {
			localDir = blobstore.DefaultLocalDir
		}
		cfg.LocalDir = promptLine(reader, "Local directory", localDir)
	default:
		return cfg, fmt.Errorf("unsupported backend %q; use Settings → Storage for GCS or Azure", backend)
	}

	return cfg, nil
}

func promptLine(reader *bufio.Reader, label, defaultValue string) string {
	if defaultValue != "" {
		fmt.Printf("%s [%s]: ", label, defaultValue)
	} else {
		fmt.Printf("%s: ", label)
	}
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultValue
	}
	return line
}

func promptSecret(label string) (string, error) {
	fmt.Printf("%s: ", label)
	raw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

func printDBTarget() {
	raw := os.Getenv("DATABASE_URL")
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		fmt.Println("Target database: (unparsed DATABASE_URL)")
		return
	}
	fmt.Printf("Target database: %s%s\n", parsed.Host, parsed.Path)
}

func printConfigSummary(cfg blobstore.Config) {
	fmt.Printf("  backend:           %s\n", cfg.Backend)
	if cfg.Bucket != "" {
		fmt.Printf("  bucket/container:  %s\n", cfg.Bucket)
	}
	if cfg.Endpoint != "" {
		fmt.Printf("  endpoint:          %s\n", cfg.Endpoint)
	}
	if cfg.Region != "" {
		fmt.Printf("  region:            %s\n", cfg.Region)
	}
	if cfg.LocalDir != "" {
		fmt.Printf("  local_dir:         %s\n", cfg.LocalDir)
	}
	if cfg.AccessKeyID != "" {
		fmt.Printf("  access_key_id:     %s\n", redactTail(cfg.AccessKeyID))
	}
	if cfg.SecretAccessKey != "" {
		fmt.Println("  secret_access_key: (set)")
	}
	if cfg.AzureAccountName != "" {
		fmt.Printf("  azure_account:     %s\n", cfg.AzureAccountName)
	}
}

func redactTail(value string) string {
	if len(value) <= 4 {
		return "…"
	}
	return "…" + value[len(value)-4:]
}
