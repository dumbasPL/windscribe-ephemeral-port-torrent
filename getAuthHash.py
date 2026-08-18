#!/usr/bin/env python3
"""Extract Windscribe's persistent auth token (authHash) across platforms.

Read-only; never modifies the Windscribe configuration.

Windows: registry HKCU\\Software\\Windscribe\\Windscribe2
Linux:   $XDG_CONFIG_HOME/Windscribe/Windscribe2.conf (INI)
macOS:   a plist under ~/Library/Preferences/

Token source priority:
  1. wsnetSettings (JSON string) -> its "authHash" member
  2. authHash (legacy plain string)
"""

import argparse
import json
import os
import sys


def resolve_source_windows():
    try:
        import winreg
    except ImportError:
        return None, None
    try:
        key = winreg.OpenKey(winreg.HKEY_CURRENT_USER, r"Software\Windscribe\Windscribe2")
    except OSError:
        return None, None
    try:
        with key:
            try:
                wsnet = winreg.QueryValueEx(key, "wsnetSettings")[0]
            except OSError:
                wsnet = None
            try:
                legacy = winreg.QueryValueEx(key, "authHash")[0]
            except OSError:
                legacy = None
        return wsnet, legacy
    except OSError:
        return None, None


def resolve_source_linux():
    base = os.environ.get("XDG_CONFIG_HOME") or os.path.join(
        os.path.expanduser("~"), ".config"
    )
    return _read_ini(os.path.join(base, "Windscribe", "Windscribe2.conf"))


def resolve_source_macos():
    prefs_dir = os.path.join(os.path.expanduser("~"), "Library", "Preferences")
    candidates = [
        "com.windscribe.client",
        "com.Windscribe.Windscribe2",
        "org.Windscribe.Windscribe2",
        "com.yourcompany.Windscribe2",
    ]
    names = [name for name in os.listdir(prefs_dir) if name.lower().startswith("windscribe")]
    for name in names:
        path = os.path.join(prefs_dir, name)
        try:
            with open(path, "rb") as fh:
                data = json.loads(fh.read().decode("utf-8", "replace"))
        except Exception:
            continue
        if not isinstance(data, dict):
            continue
        if any(k in data for k in ("wsnetSettings", "authHash")):
            return data.get("wsnetSettings"), data.get("authHash")
    for base in candidates:
        path = os.path.join(prefs_dir, base + ".plist")
        if not os.path.exists(path):
            continue
        try:
            import plistlib
            with open(path, "rb") as fh:
                data = plistlib.load(fh)
        except Exception:
            continue
        if isinstance(data, dict) and any(k in data for k in ("wsnetSettings", "authHash")):
            return data.get("wsnetSettings"), data.get("authHash")
    return None, None


def _read_ini(path):
    if not os.path.exists(path):
        return None, None
    import configparser
    parser = configparser.RawConfigParser()
    with open(path, "r", encoding="utf-8") as fh:
        parser.read_file(fh)
    wsnet = parser.get("General", "wsnetSettings") if parser.has_option("General", "wsnetSettings") else None
    legacy = parser.get("General", "authHash") if parser.has_option("General", "authHash") else None
    return wsnet, legacy


def extract(path_override=None):
    if path_override:
        wsnet, legacy = _read_ini(path_override)
    elif sys.platform.startswith("win"):
        wsnet, legacy = resolve_source_windows()
    elif sys.platform == "darwin":
        wsnet, legacy = resolve_source_macos()
    else:
        wsnet, legacy = resolve_source_linux()

    primary = None
    if wsnet:
        try:
            data = json.loads(wsnet)
            if isinstance(data, dict):
                primary = data.get("authHash")
        except json.JSONDecodeError:
            primary = None

    token = primary or legacy
    source = "wsnetSettings" if primary else ("authHash" if legacy else None)
    return token, source


def main():
    ap = argparse.ArgumentParser(description="Extract Windscribe auth token (cross-platform).")
    ap.add_argument(
        "--path",
        default=None,
        help="Read a specific INI/plist file (Linux-style) instead of auto-detection.",
    )
    ap.add_argument("--json", action="store_true", help="Emit machine-readable JSON.")
    args = ap.parse_args()

    token, source = extract(args.path)

    if not token:
        print("error: no windscribe token found (no wsnetSettings/authHash)", file=sys.stderr)
        sys.exit(1)

    if args.json:
        print(json.dumps({"authHash": token, "source": source}))
    else:
        print(f"source:   {source}")
        print(f"authHash: {token}")


if __name__ == "__main__":
    main()