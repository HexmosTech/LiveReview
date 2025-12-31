__LRC_MARKER_BEGIN__
# lrc_version: __LRC_VERSION__
# LiveReview global dispatcher for __HOOK_NAME__
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
LRC_DIR="$SCRIPT_DIR/lrc"
LRC_DISABLED_FILE=".git/lrc/disabled"
LRC_HOOK="$LRC_DIR/__HOOK_NAME__"
LOCAL_HOOK=".git/hooks/__HOOK_NAME__"

if [ -f "$LRC_DISABLED_FILE" ]; then
	LRC_DISABLED=1
else
	LRC_DISABLED=0
fi

if [ $LRC_DISABLED -eq 0 ] && [ -x "$LRC_HOOK" ]; then
	"$LRC_HOOK" "$@"
	LRC_STATUS=$?
else
	LRC_STATUS=0
fi

if [ $LRC_STATUS -ne 0 ]; then
	exit $LRC_STATUS
fi

if [ -x "$LOCAL_HOOK" ]; then
	"$LOCAL_HOOK" "$@"
	LOCAL_STATUS=$?
	if [ $LOCAL_STATUS -ne 0 ]; then
		exit $LOCAL_STATUS
	fi
fi
__LRC_MARKER_END__
