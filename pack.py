import os
import stat
import zipfile

ROOT = os.path.dirname(os.path.abspath(__file__))
VERSION = "0.1.0"
for line in open(os.path.join(ROOT, "module.prop"), encoding="utf-8"):
    if line.startswith("version="):
        VERSION = line.strip().split("=", 1)[1]

OUT = os.path.join(ROOT, "..", "apk_output", "ksu_agent_core_v%s.zip" % VERSION)
os.makedirs(os.path.dirname(OUT), exist_ok=True)

SKIP_DIRS = {"src", "tools", "deploy", "dist", "__pycache__", ".git", ".github"}
SKIP_FILES = {"build.sh", "pack.py", "initrc/agentcore.rc.disabled",
              "Dockerfile", "docker-compose.yml", ".gitignore"}


def skip(rel):
    parts = rel.split("/")
    if parts[0] in SKIP_DIRS:
        return True
    if rel in SKIP_FILES or rel.endswith(".zip"):
        return True
    return False


count = 0
with zipfile.ZipFile(OUT, "w", zipfile.ZIP_DEFLATED) as zf:
    for base, dirs, files in os.walk(ROOT):
        dirs[:] = [d for d in dirs if d not in SKIP_DIRS]
        for name in files:
            full = os.path.join(base, name)
            rel = os.path.relpath(full, ROOT).replace("\\", "/")
            if skip(rel):
                continue
            info = zipfile.ZipInfo.from_file(full, rel)
            executable = rel.endswith(".sh") or rel == "bin/agentcore"
            info.external_attr = (stat.S_IFREG | (0o755 if executable else 0o644)) << 16
            info.compress_type = zipfile.ZIP_DEFLATED
            with open(full, "rb") as fh:
                zf.writestr(info, fh.read())
            count += 1

print("built: %s (%d files)" % (OUT, count))
