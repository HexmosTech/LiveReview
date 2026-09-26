package seed_demo

import (
	"os"
	"strings"
)

// Enabled reports whether demo seeding may run in this process, and if not, why.
// Both keys are required:
//   - LIVEREVIEW_IS_CLOUD=true: never on a self-hosted install.
//   - SEED_DEMO_ENABLED=true: set only in .env.prod (the Makefile's ensure-seed-demo-env
//     step adds it if missing), which `make raw-deploy` / `make raw-deploy-backend` ship
//     to the prod server as .env. Local .env files never have it, so a local cloud-mode
//     dev setup still never seeds.
func Enabled() (bool, string) {
	if !envBoolTrue(os.Getenv("LIVEREVIEW_IS_CLOUD")) {
		return false, "LIVEREVIEW_IS_CLOUD is not true (self-hosted)"
	}
	if !envBoolTrue(os.Getenv("SEED_DEMO_ENABLED")) {
		return false, "SEED_DEMO_ENABLED is not true (only set in .env.prod)"
	}
	return true, ""
}

// envBoolTrue matches internal/api's getEnvBool convention: "true" or "1".
func envBoolTrue(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	return v == "true" || v == "1"
}
