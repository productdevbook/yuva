import os
import re
import subprocess
import sys

version, tag, image = sys.argv[1], sys.argv[2], sys.argv[3]

changelog = open("CHANGELOG.md", encoding="utf-8").read()
section = re.search(
    r"^## \[" + re.escape(version) + r"\][^\n]*\n(.*?)(?=^## \[|^\[[^\]]+\]: |\Z)",
    changelog,
    re.S | re.M,
)
if not section or not section.group(1).strip():
    sys.exit(f"no CHANGELOG.md section for {version}")
notes = section.group(1).strip()

warning = ""
if version.startswith("0."):
    warning = (
        "> [!WARNING]\n"
        "> **Pre-alpha.** Yuva is young: the API and database schema can still change between\n"
        "> releases. Try it, read it, tell us what breaks — but keep real customer data out of it for now."
    )

previous = subprocess.run(
    ["git", "describe", "--tags", "--abbrev=0", "--match", "v*", f"{tag}^"],
    capture_output=True,
    text=True,
).stdout.strip()
compare = (
    f"**Full changes:** https://github.com/productdevbook/yuva/compare/{previous}...{tag}"
    if previous
    else f"**First release.** Every commit is in https://github.com/productdevbook/yuva/commits/{tag}"
)

out = open(".github/release-template.md", encoding="utf-8").read()
for key, value in {
    "{{WARNING}}": warning,
    "{{NOTES}}": notes,
    "{{COMPARE}}": compare,
    "{{VERSION}}": version,
    "{{TAG}}": tag,
    "{{IMAGE}}": image,
}.items():
    out = out.replace(key, value)
sys.stdout.write(re.sub(r"\n{3,}", "\n\n", out))
