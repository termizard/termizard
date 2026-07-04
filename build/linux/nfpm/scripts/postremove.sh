#!/bin/sh
set -e

# Remove desktop shortcuts created by postinstall.
for user_home in /home/*; do
  [ -d "$user_home" ] || continue
  for desktop_dir in "$user_home/Desktop" "$user_home/Рабочий стол"; do
    [ -f "$desktop_dir/termizard.desktop" ] && rm -f "$desktop_dir/termizard.desktop"
  done
done

if command -v update-desktop-database >/dev/null 2>&1; then
  update-desktop-database -q /usr/share/applications
fi

exit 0
