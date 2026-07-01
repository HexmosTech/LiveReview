package lrcconfig

// BundleFromFiles wraps a raw files map (as returned by lrcfetch.Provider)
// into a Bundle ready for BuildRulesBundle / LoadIgnorePatterns / FilterDiffs.
func BundleFromFiles(files map[string][]byte) Bundle {
	return Bundle{Files: files}
}
