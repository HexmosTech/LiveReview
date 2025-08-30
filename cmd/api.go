package cmd

import (
	"fmt"
	"os"

	"github.com/livereview/internal/api"
	"github.com/urfave/cli/v2"
)

// GetVersionInfo returns version information from main package variables
// These variables should be set by main package
var (
	Version   = "development"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

// APICommand returns the CLI command for starting the API server
func APICommand() *cli.Command {
	return &cli.Command{
		Name:  "api",
		Usage: "Start the LiveReview API server",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:    "port",
				Aliases: []string{"p"},
				Usage:   "Port for the API server",
				Value:   8888,
			},
			&cli.StringFlag{
				Name:    "set-admin-password",
				Aliases: []string{"set-password"},
				Usage:   "Set the admin password for the instance",
			},
			&cli.BoolFlag{
				Name:    "force",
				Aliases: []string{"f"},
				Usage:   "Force the password operation even if a password is already set",
			},
			&cli.StringFlag{
				Name:    "reset-admin-password-old",
				Aliases: []string{"old-password"},
				Usage:   "The current admin password (used with --reset-admin-password-new)",
			},
			&cli.StringFlag{
				Name:    "reset-admin-password-new",
				Aliases: []string{"new-password"},
				Usage:   "The new admin password (used with --reset-admin-password-old)",
			},
			&cli.StringFlag{
				Name:    "verify-admin-password",
				Aliases: []string{"verify-password"},
				Usage:   "Verify if the provided admin password is correct",
			},
			&cli.StringFlag{
				Name:    "check-admin-password-status",
				Aliases: []string{"check-password"},
				Usage:   "Check if an admin password has been set",
			},
			&cli.BoolFlag{
				Name:  "get-prod-url",
				Usage: "Get the production URL for LiveReview",
			},
			&cli.StringFlag{
				Name:  "set-prod-url",
				Usage: "Set the production URL for LiveReview",
			},
		},
		Action: func(c *cli.Context) error {
			port := c.Int("port")

			// Create version info from global variables
			versionInfo := &api.VersionInfo{
				Version:   Version,
				GitCommit: GitCommit,
				BuildTime: BuildTime,
				Dirty:     false, // This will be set properly during build
			}

			// Create server instance
			server, err := api.NewServer(port, versionInfo)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error initializing server: %v\n", err)
				return err
			}

			// Handle set admin password
			if password := c.String("set-admin-password"); password != "" {
				force := c.Bool("force")
				if force {
					fmt.Println("Setting admin password (with force)...")
				} else {
					fmt.Println("Setting admin password...")
				}

				if err := server.SetAdminPasswordDirectly(password, force); err != nil {
					fmt.Fprintf(os.Stderr, "Error setting admin password: %v\n", err)
					return err
				}
				fmt.Println("Admin password set successfully.")
				return nil
			}

			// Handle reset admin password
			oldPassword := c.String("reset-admin-password-old")
			newPassword := c.String("reset-admin-password-new")

			if oldPassword != "" || newPassword != "" {
				// Validate both old and new passwords are provided
				if oldPassword == "" {
					fmt.Fprintf(os.Stderr, "Error: --reset-admin-password-old is required when resetting password\n")
					return fmt.Errorf("missing old password")
				}
				if newPassword == "" {
					fmt.Fprintf(os.Stderr, "Error: --reset-admin-password-new is required when resetting password\n")
					return fmt.Errorf("missing new password")
				}

				fmt.Println("Resetting admin password...")
				if err := server.ResetAdminPasswordDirectly(oldPassword, newPassword); err != nil {
					fmt.Fprintf(os.Stderr, "Error resetting admin password: %v\n", err)
					return err
				}
				fmt.Println("Admin password reset successfully.")
				return nil
			}

			// Handle verify admin password
			if password := c.String("verify-admin-password"); password != "" {
				fmt.Println("Verifying admin password...")
				valid, err := server.VerifyAdminPasswordDirectly(password)
				if err != nil {
					// Just return the error, don't print it
					return err
				}

				if valid {
					fmt.Println("Password is valid.")
				} else {
					// Just return the error, don't print it
					return fmt.Errorf("invalid password")
				}
				return nil
			}

			// Handle check admin password status
			if c.Bool("check-admin-password-status") {
				fmt.Println("Checking if admin password is set...")
				isSet, err := server.CheckAdminPasswordStatusDirectly()
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error checking password status: %v\n", err)
					return err
				}

				if isSet {
					fmt.Println("Admin password is set.")
				} else {
					fmt.Println("No admin password has been set yet.")
				}
				return nil
			}

			// Handle get production URL
			if c.Bool("get-prod-url") {
				fmt.Println("Getting production URL...")
				url, err := server.GetProductionURLDirectly()
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error getting production URL: %v\n", err)
					return err
				}
				fmt.Printf("Production URL: %s\n", url)
				return nil
			}

			// Handle set production URL
			if url := c.String("set-prod-url"); url != "" {
				fmt.Println("Setting production URL...")
				if err := server.UpdateProductionURLDirectly(url); err != nil {
					fmt.Fprintf(os.Stderr, "Error setting production URL: %v\n", err)
					return err
				}
				fmt.Println("Production URL set successfully.")
				return nil
			}

			// Start the API server if no password management flags were used
			fmt.Printf("Starting LiveReview API server on port %d...\n", port)
			return server.Start()
		},
	}
}
