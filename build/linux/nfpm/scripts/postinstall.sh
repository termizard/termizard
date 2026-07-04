#!/bin/sh
set -e

APP_DESKTOP="/usr/share/applications/termizard.desktop"

# Register the app in desktop menus (application launcher shortcut).
if command -v update-desktop-database >/dev/null 2>&1; then
  update-desktop-database -q /usr/share/applications
fi

if command -v gtk-update-icon-cache >/dev/null 2>&1; then
  gtk-update-icon-cache -q /usr/share/icons/hicolor 2>/dev/null || true
fi

# Desktop shortcut for each user with a Desktop folder.
if [ -f "$APP_DESKTOP" ]; then
  for user_home in /home/*; do
    [ -d "$user_home" ] || continue
    desktop_dir="$user_home/Desktop"
    [ -d "$desktop_dir" ] || desktop_dir="$user_home/Рабочий стол"
    [ -d "$desktop_dir" ] || continue
    user=$(basename "$user_home")
    if id "$user" >/dev/null 2>&1; then
      cp "$APP_DESKTOP" "$desktop_dir/termizard.desktop"
      chown "$user:$user" "$desktop_dir/termizard.desktop"
      chmod +x "$desktop_dir/termizard.desktop"
    fi
  done
fi

if command -v update-mime-database >/dev/null 2>&1; then
  update-mime-database -n /usr/share/mime
fi

exit 0
