package license

import "time"

const licenseTimeOffsetDays = 0

func licenseNow() time.Time {
	now := time.Now()
	if licenseTimeOffsetDays == 0 {
		return now
	}

	return now.AddDate(0, 0, licenseTimeOffsetDays)
}
