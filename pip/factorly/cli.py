"""Factorly CLI wrapper — downloads and runs the Go binary."""

import os
import platform
import stat
import subprocess
import sys
import urllib.request

from factorly import __version__

REPO = "factorly-dev/factorly-cli"

PLATFORM_MAP = {
    "Darwin": "darwin",
    "Linux": "linux",
    "Windows": "windows",
}

ARCH_MAP = {
    "x86_64": "amd64",
    "AMD64": "amd64",
    "aarch64": "arm64",
    "arm64": "arm64",
}


def get_bin_dir():
    """Return the directory where the binary is stored."""
    return os.path.join(os.path.dirname(os.path.abspath(__file__)), "bin")


def get_binary_path():
    """Return the full path to the factorly binary."""
    ext = ".exe" if platform.system() == "Windows" else ""
    return os.path.join(get_bin_dir(), f"factorly{ext}")


def get_binary_name():
    """Return the platform-specific binary name for download."""
    plat = PLATFORM_MAP.get(platform.system())
    arch = ARCH_MAP.get(platform.machine())

    if not plat:
        print(f"Unsupported platform: {platform.system()}", file=sys.stderr)
        sys.exit(1)
    if not arch:
        print(f"Unsupported architecture: {platform.machine()}", file=sys.stderr)
        sys.exit(1)

    ext = ".exe" if platform.system() == "Windows" else ""
    return f"factorly-{plat}-{arch}{ext}"


def is_installed():
    """Check if the correct version is already installed."""
    binary = get_binary_path()
    if not os.path.exists(binary):
        return False
    try:
        result = subprocess.run(
            [binary, "version"],
            capture_output=True,
            text=True,
            timeout=5,
        )
        return __version__ in result.stdout
    except Exception:
        return False


def download_binary():
    """Download the factorly binary from GitHub Releases."""
    binary_name = get_binary_name()
    url = f"https://github.com/{REPO}/releases/download/v{__version__}/{binary_name}"

    print(f"Downloading factorly {__version__} for {platform.system()}/{platform.machine()}...")
    print(f"  {url}")

    bin_dir = get_bin_dir()
    os.makedirs(bin_dir, exist_ok=True)
    binary_path = get_binary_path()

    try:
        req = urllib.request.Request(url, headers={"User-Agent": "factorly-pip-installer"})
        with urllib.request.urlopen(req) as response:
            data = response.read()
    except urllib.error.HTTPError as e:
        print(f"Download failed: HTTP {e.code}", file=sys.stderr)
        print(f"", file=sys.stderr)
        print(f"Install manually from:", file=sys.stderr)
        print(f"  https://github.com/{REPO}/releases/tag/v{__version__}", file=sys.stderr)
        sys.exit(1)
    except Exception as e:
        print(f"Download failed: {e}", file=sys.stderr)
        sys.exit(1)

    with open(binary_path, "wb") as f:
        f.write(data)

    # Make executable
    if platform.system() != "Windows":
        st = os.stat(binary_path)
        os.chmod(binary_path, st.st_mode | stat.S_IEXEC | stat.S_IXGRP | stat.S_IXOTH)

    print(f"Installed factorly to {binary_path}")


def main():
    """Entry point: ensure binary exists, then run it."""
    if not is_installed():
        download_binary()

    binary = get_binary_path()
    if not os.path.exists(binary):
        print("factorly binary not found after download attempt.", file=sys.stderr)
        print("Run: pip install --force-reinstall factorly", file=sys.stderr)
        sys.exit(127)

    result = subprocess.run([binary] + sys.argv[1:])
    sys.exit(result.returncode)


if __name__ == "__main__":
    main()
