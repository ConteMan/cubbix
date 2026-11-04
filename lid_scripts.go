package main

// All authorization programs are immutable; no untrusted input is interpolated.
const (
	lidInstallScript = `set -eu
PATH=/usr/bin:/bin:/usr/sbin:/sbin
export PATH
step=copy
trap 'rc=$?; printf "cubbix-step=%s\ncubbix-exit=%s\n" "$step" "$rc" >&2' EXIT
printf 'cubbix-step=copy\n'
/usr/bin/install -d -m 0755 -o root -g wheel /Library/PrivilegedHelperTools
/usr/bin/install -m 0755 -o root -g wheel /Applications/Cubbix.app/Contents/Resources/bin/cubbix-helper /Library/PrivilegedHelperTools/com.conteman.cubbix.helper.new
step=verify
printf 'cubbix-step=verify\n'
verify_rc=0
/usr/bin/codesign -v -R="anchor apple generic and certificate leaf[subject.OU] = \"P93GN4N3DC\" and identifier \"cubbix-helper\"" /Library/PrivilegedHelperTools/com.conteman.cubbix.helper.new || verify_rc=$?
if [ "$verify_rc" -ne 0 ]; then
  printf 'cubbix-command-exit=%s\n' "$verify_rc" >&2
  /bin/rm -f /Library/PrivilegedHelperTools/com.conteman.cubbix.helper.new
  exit 1
fi
step=replace
printf 'cubbix-step=replace\n'
/bin/mv -f /Library/PrivilegedHelperTools/com.conteman.cubbix.helper.new /Library/PrivilegedHelperTools/com.conteman.cubbix.helper
step=plist
printf 'cubbix-step=plist\n'
/bin/cat > /Library/LaunchDaemons/com.conteman.cubbix.helper.plist <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.conteman.cubbix.helper</string>
  <key>Program</key><string>/Library/PrivilegedHelperTools/com.conteman.cubbix.helper</string>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>ExitTimeOut</key><integer>15</integer>
  <key>ThrottleInterval</key><integer>10</integer>
  <key>ProcessType</key><string>Background</string>
  <key>AssociatedBundleIdentifiers</key><array><string>com.conteman.cubbix</string></array>
</dict>
</plist>
PLIST
/usr/sbin/chown root:wheel /Library/LaunchDaemons/com.conteman.cubbix.helper.plist
/bin/chmod 0644 /Library/LaunchDaemons/com.conteman.cubbix.helper.plist
step=bootout
printf 'cubbix-step=bootout\n'
bootout_failed=0
/bin/launchctl bootout system/com.conteman.cubbix.helper || bootout_failed=$?
if [ "$bootout_failed" -ne 0 ]; then
  printf 'cubbix-command-exit=%s\n' "$bootout_failed" >&2
  if /bin/launchctl print system/com.conteman.cubbix.helper >/dev/null 2>&1; then
    step=terminate
    printf 'cubbix-step=terminate\n'
    /bin/launchctl kill SIGTERM system/com.conteman.cubbix.helper || true
    /bin/launchctl bootout system/com.conteman.cubbix.helper || true
  fi
fi
step=unload-wait
printf 'cubbix-step=unload-wait\n'
unload_attempt=0
while /bin/launchctl print system/com.conteman.cubbix.helper >/dev/null 2>&1; do
  if [ "$unload_attempt" -ge 300 ]; then
    printf 'cubbix-error=unload-timeout\n' >&2
    exit 1
  fi
  unload_attempt=$((unload_attempt + 1))
  /bin/sleep 0.1
done
step=bootstrap
printf 'cubbix-step=bootstrap\n'
bootstrap_attempt=0
while :; do
  bootstrap_rc=0
  /bin/launchctl bootstrap system /Library/LaunchDaemons/com.conteman.cubbix.helper.plist || bootstrap_rc=$?
  if [ "$bootstrap_rc" -eq 0 ]; then break; fi
  printf 'cubbix-command-exit=%s\n' "$bootstrap_rc" >&2
  bootstrap_attempt=$((bootstrap_attempt + 1))
  if [ "$bootstrap_attempt" -ge 5 ]; then
    printf 'cubbix-error=bootstrap-failed\n' >&2
    exit 1
  fi
  /bin/sleep 1
done
step=confirm
printf 'cubbix-step=confirm\n'
if ! /bin/launchctl print system/com.conteman.cubbix.helper >/dev/null 2>&1; then
  printf 'cubbix-error=confirm-failed\n' >&2
  exit 1
fi
`
	lidInstallScriptApple = "do shell script \"set -eu\\nPATH=/usr/bin:/bin:/usr/sbin:/sbin\\nexport PATH\\nstep=copy\\ntrap 'rc=$?; printf \\\"cubbix-step=%s\\\\ncubbix-exit=%s\\\\n\\\" \\\"$step\\\" \\\"$rc\\\" >&2' EXIT\\nprintf 'cubbix-step=copy\\\\n'\\n/usr/bin/install -d -m 0755 -o root -g wheel /Library/PrivilegedHelperTools\\n/usr/bin/install -m 0755 -o root -g wheel /Applications/Cubbix.app/Contents/Resources/bin/cubbix-helper /Library/PrivilegedHelperTools/com.conteman.cubbix.helper.new\\nstep=verify\\nprintf 'cubbix-step=verify\\\\n'\\nverify_rc=0\\n/usr/bin/codesign -v -R=\\\"anchor apple generic and certificate leaf[subject.OU] = \\\\\\\"P93GN4N3DC\\\\\\\" and identifier \\\\\\\"cubbix-helper\\\\\\\"\\\" /Library/PrivilegedHelperTools/com.conteman.cubbix.helper.new || verify_rc=$?\\nif [ \\\"$verify_rc\\\" -ne 0 ]; then\\n  printf 'cubbix-command-exit=%s\\\\n' \\\"$verify_rc\\\" >&2\\n  /bin/rm -f /Library/PrivilegedHelperTools/com.conteman.cubbix.helper.new\\n  exit 1\\nfi\\nstep=replace\\nprintf 'cubbix-step=replace\\\\n'\\n/bin/mv -f /Library/PrivilegedHelperTools/com.conteman.cubbix.helper.new /Library/PrivilegedHelperTools/com.conteman.cubbix.helper\\nstep=plist\\nprintf 'cubbix-step=plist\\\\n'\\n/bin/cat > /Library/LaunchDaemons/com.conteman.cubbix.helper.plist <<'PLIST'\\n<?xml version=\\\"1.0\\\" encoding=\\\"UTF-8\\\"?>\\n<!DOCTYPE plist PUBLIC \\\"-//Apple//DTD PLIST 1.0//EN\\\" \\\"http://www.apple.com/DTDs/PropertyList-1.0.dtd\\\">\\n<plist version=\\\"1.0\\\">\\n<dict>\\n  <key>Label</key><string>com.conteman.cubbix.helper</string>\\n  <key>Program</key><string>/Library/PrivilegedHelperTools/com.conteman.cubbix.helper</string>\\n  <key>RunAtLoad</key><true/>\\n  <key>KeepAlive</key><true/>\\n  <key>ExitTimeOut</key><integer>15</integer>\\n  <key>ThrottleInterval</key><integer>10</integer>\\n  <key>ProcessType</key><string>Background</string>\\n  <key>AssociatedBundleIdentifiers</key><array><string>com.conteman.cubbix</string></array>\\n</dict>\\n</plist>\\nPLIST\\n/usr/sbin/chown root:wheel /Library/LaunchDaemons/com.conteman.cubbix.helper.plist\\n/bin/chmod 0644 /Library/LaunchDaemons/com.conteman.cubbix.helper.plist\\nstep=bootout\\nprintf 'cubbix-step=bootout\\\\n'\\nbootout_failed=0\\n/bin/launchctl bootout system/com.conteman.cubbix.helper || bootout_failed=$?\\nif [ \\\"$bootout_failed\\\" -ne 0 ]; then\\n  printf 'cubbix-command-exit=%s\\\\n' \\\"$bootout_failed\\\" >&2\\n  if /bin/launchctl print system/com.conteman.cubbix.helper >/dev/null 2>&1; then\\n    step=terminate\\n    printf 'cubbix-step=terminate\\\\n'\\n    /bin/launchctl kill SIGTERM system/com.conteman.cubbix.helper || true\\n    /bin/launchctl bootout system/com.conteman.cubbix.helper || true\\n  fi\\nfi\\nstep=unload-wait\\nprintf 'cubbix-step=unload-wait\\\\n'\\nunload_attempt=0\\nwhile /bin/launchctl print system/com.conteman.cubbix.helper >/dev/null 2>&1; do\\n  if [ \\\"$unload_attempt\\\" -ge 300 ]; then\\n    printf 'cubbix-error=unload-timeout\\\\n' >&2\\n    exit 1\\n  fi\\n  unload_attempt=$((unload_attempt + 1))\\n  /bin/sleep 0.1\\ndone\\nstep=bootstrap\\nprintf 'cubbix-step=bootstrap\\\\n'\\nbootstrap_attempt=0\\nwhile :; do\\n  bootstrap_rc=0\\n  /bin/launchctl bootstrap system /Library/LaunchDaemons/com.conteman.cubbix.helper.plist || bootstrap_rc=$?\\n  if [ \\\"$bootstrap_rc\\\" -eq 0 ]; then break; fi\\n  printf 'cubbix-command-exit=%s\\\\n' \\\"$bootstrap_rc\\\" >&2\\n  bootstrap_attempt=$((bootstrap_attempt + 1))\\n  if [ \\\"$bootstrap_attempt\\\" -ge 5 ]; then\\n    printf 'cubbix-error=bootstrap-failed\\\\n' >&2\\n    exit 1\\n  fi\\n  /bin/sleep 1\\ndone\\nstep=confirm\\nprintf 'cubbix-step=confirm\\\\n'\\nif ! /bin/launchctl print system/com.conteman.cubbix.helper >/dev/null 2>&1; then\\n  printf 'cubbix-error=confirm-failed\\\\n' >&2\\n  exit 1\\nfi\\n\" with administrator privileges"
	lidUninstallScript    = `set -eu
/bin/launchctl bootout system/com.conteman.cubbix.helper 2>/dev/null || true
/bin/rm -f /Library/LaunchDaemons/com.conteman.cubbix.helper.plist
/bin/rm -f /Library/PrivilegedHelperTools/com.conteman.cubbix.helper
/bin/rm -rf "/Library/Application Support/com.conteman.cubbix.helper"
`
	lidUninstallScriptApple = "do shell script \"set -eu\\n/bin/launchctl bootout system/com.conteman.cubbix.helper 2>/dev/null || true\\n/bin/rm -f /Library/LaunchDaemons/com.conteman.cubbix.helper.plist\\n/bin/rm -f /Library/PrivilegedHelperTools/com.conteman.cubbix.helper\\n/bin/rm -rf \\\"/Library/Application Support/com.conteman.cubbix.helper\\\"\\n\" with administrator privileges"
	lidForceScript          = `/usr/bin/pmset -a disablesleep 0`
	lidForceScriptApple     = "do shell script \"/usr/bin/pmset -a disablesleep 0\" with administrator privileges"
	lidUpdateScript         = lidInstallScript
)
