#!/usr/bin/env bash
set -euo pipefail

cp SKILL.md docs/SKILL.md
cp SKILL.md skills/nebu/SKILL.md

echo "✓ synced SKILL.md -> docs/SKILL.md, skills/nebu/SKILL.md"
