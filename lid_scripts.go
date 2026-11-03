package main

// All authorization programs are immutable; no untrusted input is interpolated.
const (
	lidInstallScript = `set -eu
PATH=/usr/bin:/bin:/usr/sbin:/sbin
export PATH
/usr/bin/install -d -m 0755 -o root -g wheel /Library/PrivilegedHelperTools
/usr/bin/install -m 0755 -o root -g wheel /Applications/Cubbix.app/Contents/Resources/bin/cubbix-helper /Library/PrivilegedHelperTools/com.conteman.cubbix.helper.new
if ! /usr/bin/codesign -v -R="anchor apple generic and certificate leaf[subject.OU] = \"P93GN4N3DC\" and identifier \"cubbix-helper\"" /Library/PrivilegedHelperTools/com.conteman.cubbix.helper.new; then
  /bin/rm -f /Library/PrivilegedHelperTools/com.conteman.cubbix.helper.new
  exit 1
fi
/bin/mv -f /Library/PrivilegedHelperTools/com.conteman.cubbix.helper.new /Library/PrivilegedHelperTools/com.conteman.cubbix.helper
/bin/cat > /Library/LaunchDaemons/com.conteman.cubbix.helper.plist <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.conteman.cubbix.helper</string>
  <key>Program</key><string>/Library/PrivilegedHelperTools/com.conteman.cubbix.helper</string>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>ThrottleInterval</key><integer>10</integer>
  <key>ProcessType</key><string>Background</string>
  <key>AssociatedBundleIdentifiers</key><array><string>com.conteman.cubbix</string></array>
</dict>
</plist>
PLIST
/bin/chown root:wheel /Library/LaunchDaemons/com.conteman.cubbix.helper.plist
/bin/chmod 0644 /Library/LaunchDaemons/com.conteman.cubbix.helper.plist
/bin/launchctl bootout system/com.conteman.cubbix.helper 2>/dev/null || true
/bin/launchctl bootstrap system /Library/LaunchDaemons/com.conteman.cubbix.helper.plist
`
	lidInstallScriptApple = "do shell script \"set -eu\\nPATH=/usr/bin:/bin:/usr/sbin:/sbin\\nexport PATH\\n/usr/bin/install -d -m 0755 -o root -g wheel /Library/PrivilegedHelperTools\\n/usr/bin/install -m 0755 -o root -g wheel /Applications/Cubbix.app/Contents/Resources/bin/cubbix-helper /Library/PrivilegedHelperTools/com.conteman.cubbix.helper.new\\nif ! /usr/bin/codesign -v -R=\\\"anchor apple generic and certificate leaf[subject.OU] = \\\\\\\"P93GN4N3DC\\\\\\\" and identifier \\\\\\\"cubbix-helper\\\\\\\"\\\" /Library/PrivilegedHelperTools/com.conteman.cubbix.helper.new; then\\n  /bin/rm -f /Library/PrivilegedHelperTools/com.conteman.cubbix.helper.new\\n  exit 1\\nfi\\n/bin/mv -f /Library/PrivilegedHelperTools/com.conteman.cubbix.helper.new /Library/PrivilegedHelperTools/com.conteman.cubbix.helper\\n/bin/cat > /Library/LaunchDaemons/com.conteman.cubbix.helper.plist <<'PLIST'\\n<?xml version=\\\"1.0\\\" encoding=\\\"UTF-8\\\"?>\\n<!DOCTYPE plist PUBLIC \\\"-//Apple//DTD PLIST 1.0//EN\\\" \\\"http://www.apple.com/DTDs/PropertyList-1.0.dtd\\\">\\n<plist version=\\\"1.0\\\">\\n<dict>\\n  <key>Label</key><string>com.conteman.cubbix.helper</string>\\n  <key>Program</key><string>/Library/PrivilegedHelperTools/com.conteman.cubbix.helper</string>\\n  <key>RunAtLoad</key><true/>\\n  <key>KeepAlive</key><true/>\\n  <key>ThrottleInterval</key><integer>10</integer>\\n  <key>ProcessType</key><string>Background</string>\\n  <key>AssociatedBundleIdentifiers</key><array><string>com.conteman.cubbix</string></array>\\n</dict>\\n</plist>\\nPLIST\\n/bin/chown root:wheel /Library/LaunchDaemons/com.conteman.cubbix.helper.plist\\n/bin/chmod 0644 /Library/LaunchDaemons/com.conteman.cubbix.helper.plist\\n/bin/launchctl bootout system/com.conteman.cubbix.helper 2>/dev/null || true\\n/bin/launchctl bootstrap system /Library/LaunchDaemons/com.conteman.cubbix.helper.plist\\n\" with administrator privileges"
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
