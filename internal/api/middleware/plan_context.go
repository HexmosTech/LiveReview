package middleware

import (
	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/license"
)

const PlanContextKey = "plan_context"

type PlanContext struct {
	PlanType license.PlanType
	Limits   license.PlanLimits
}

// BuildPlanContext resolves plan metadata once and stores it in request context
// so downstream handlers can use a consistent view.
func BuildPlanContext() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			planTypeStr, _ := c.Get("plan_type").(string)
			if planTypeStr == "" {
				planTypeStr = string(license.PlanFree)
			}

			planType := license.PlanType(planTypeStr)
			if !planType.IsValid() {
				planType = license.PlanFree
			}

			ctx := PlanContext{
				PlanType: planType,
				Limits:   planType.GetLimits(),
			}

			c.Set(PlanContextKey, ctx)
			return next(c)
		}
	}
}
