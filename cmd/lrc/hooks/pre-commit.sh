__LRC_MARKER_BEGIN__
# lrc_version: __LRC_VERSION__
# This section is managed by LiveReview CLI (lrc)
# Manual changes within markers will be lost on hook updates

# Detect interactive terminal (stdout check; git redirects stdin)
if [ -t 1 ]; then
	echo "LiveReview pre-commit: interactive environment detected; no-op"
	exit 0
fi

echo "LiveReview pre-commit: non-interactive environment detected; aborting commit"
exit 1
__LRC_MARKER_END__
