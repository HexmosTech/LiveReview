package seed_demo

import (
	"crypto/rand"
	"encoding/hex"
	"math/big"
	"time"
)

// randomCommitSHA generates a fake-but-well-formed 40-hex-char git commit SHA, standing
// in for "another commit was pushed to the same PR" since we don't have a real new commit.
func randomCommitSHA() (string, error) {
	buf := make([]byte, 20)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// randomTimeToday returns a random instant within the given day (local time), keeping
// hour-of-day within a plausible working window (7am-11pm) so activity looks organic
// rather than clustered at the exact minute the cron job happened to fire.
func randomTimeToday(day time.Time) (time.Time, error) {
	startOfDay := time.Date(day.Year(), day.Month(), day.Day(), 7, 0, 0, 0, day.Location())
	const windowSeconds = 16 * 60 * 60 // 7am-11pm
	n, err := rand.Int(rand.Reader, big.NewInt(windowSeconds))
	if err != nil {
		return time.Time{}, err
	}
	return startOfDay.Add(time.Duration(n.Int64()) * time.Second), nil
}

// jitterInt64 scales v by a random factor in [1-pct, 1+pct], keeping it at least 1 when v > 0.
func jitterInt64(v int64, pct float64) (int64, error) {
	if v <= 0 {
		return v, nil
	}
	factor, err := jitterFactor(pct)
	if err != nil {
		return 0, err
	}
	out := int64(float64(v) * factor)
	if out < 1 {
		out = 1
	}
	return out, nil
}

// jitterFloat64 scales v by a random factor in [1-pct, 1+pct].
func jitterFloat64(v float64, pct float64) (float64, error) {
	factor, err := jitterFactor(pct)
	if err != nil {
		return 0, err
	}
	return v * factor, nil
}

// jitterFactor returns a random float in [1-pct, 1+pct], e.g. pct=0.2 -> [0.8, 1.2].
func jitterFactor(pct float64) (float64, error) {
	const resolution = 1000
	n, err := rand.Int(rand.Reader, big.NewInt(2*resolution+1))
	if err != nil {
		return 1, err
	}
	// n in [0, 2*resolution] -> offset in [-pct, +pct]
	offset := (float64(n.Int64())/resolution - 1) * pct
	return 1 + offset, nil
}

// pickN chooses up to n distinct random indices from a pool of the given size.
func pickN(poolSize, n int) ([]int, error) {
	if n > poolSize {
		n = poolSize
	}
	picked := map[int]bool{}
	out := make([]int, 0, n)
	for len(out) < n {
		bi, err := rand.Int(rand.Reader, big.NewInt(int64(poolSize)))
		if err != nil {
			return nil, err
		}
		i := int(bi.Int64())
		if picked[i] {
			continue
		}
		picked[i] = true
		out = append(out, i)
	}
	return out, nil
}

// randomCount returns a random int in [min, max] inclusive.
func randomCount(min, max int) (int, error) {
	if max <= min {
		return min, nil
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max-min+1)))
	if err != nil {
		return 0, err
	}
	return min + int(n.Int64()), nil
}
