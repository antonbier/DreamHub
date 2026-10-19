# -*- coding: utf-8 -*-
# The backend's port lives in one place: DREAMHUB_LISTEN_ADDR in
# /etc/default/dreamhub, which the systemd unit reads (EnvironmentFile).
# This module reads and writes that file, so the launcher, the backend and
# the mDNS advertisement always agree; nothing is cached in enigma2's own
# settings.
import os
import re

DEFAULT_PORT = 8765
ENV_FILE = "/etc/default/dreamhub"
AVAHI_FILE = "/etc/avahi/services/dreamhub.service"
_KEY = "DREAMHUB_LISTEN_ADDR"


def readPort():
    try:
        with open(ENV_FILE) as f:
            lines = f.read().splitlines()
    except (IOError, OSError):
        return DEFAULT_PORT
    port = DEFAULT_PORT
    for line in lines:
        m = re.match(r"^\s*%s=.*:(\d+)\s*$" % _KEY, line)
        if m:
            port = int(m.group(1))
    return port


def writePort(port):
    """Writes the port to ENV_FILE and the mDNS file. Raises IOError/OSError."""
    kept = []
    try:
        with open(ENV_FILE) as f:
            kept = [l for l in f.read().splitlines() if not re.match(r"^\s*%s=" % _KEY, l)]
    except (IOError, OSError):
        pass
    kept.append("%s=:%d" % (_KEY, port))
    tmp = ENV_FILE + ".tmp"
    with open(tmp, "w") as f:
        f.write("\n".join(kept) + "\n")
    os.rename(tmp, ENV_FILE)
    if os.path.exists(AVAHI_FILE):
        with open(AVAHI_FILE) as f:
            xml = f.read()
        with open(AVAHI_FILE, "w") as f:
            f.write(re.sub(r"<port>\d+</port>", "<port>%d</port>" % port, xml))


# Run after a confirmed change: DreamHub picks up the new port, avahi the new
# advertisement. Fire and forget, so the GUI never waits on systemctl.
RESTART_CMD = "systemctl restart dreamhub; systemctl reload avahi-daemon"
