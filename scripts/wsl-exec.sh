#!/bin/sh
# The `go test -exec` helper of `make test-windows` in WSL: runs a Windows
# test binary on the Windows host, as CI's Windows runner does. Its PATH
# holds Windows folders only, Git for Windows' bin first (sh and git, as on
# the runner); its user profile (USERPROFILE, APPDATA, LOCALAPPDATA) is a
# throwaway folder in IMUX_WINTEST_PROFILE, never the real one.
set -eu
exe=$1
shift
profile=${IMUX_WINTEST_PROFILE:?run it through make test-windows}
mkdir -p "$profile/home" "$profile/roaming" "$profile/local"

winpath=
if git=$(command -v git.exe); then
	bin=$(dirname "$(dirname "$git")")/bin
	if [ -d "$bin" ]; then
		winpath=$bin
	fi
fi
set -f
IFS=:
for d in $PATH; do
	case $d in
	/mnt/[a-z]/*) winpath=${winpath:+$winpath:}$d ;;
	esac
done
unset IFS
set +f
# Through interop the binary's argv[0] is its bare name; tests that run
# themselves again (exec.Command(os.Args[0], ...)) find it on PATH.
winpath=$winpath:$(dirname "$exe")

USERPROFILE=$profile/home
APPDATA=$profile/roaming
LOCALAPPDATA=$profile/local
WSLENV="PATH/l:USERPROFILE/p:APPDATA/p:LOCALAPPDATA/p${WSLENV:+:$WSLENV}"
PATH=$winpath
export USERPROFILE APPDATA LOCALAPPDATA WSLENV PATH
exec "$exe" "$@"
